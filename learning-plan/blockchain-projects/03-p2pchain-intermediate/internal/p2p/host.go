// Package p2p is the network layer: a libp2p host, GossipSub for spreading
// blocks and transactions, and a request/response protocol for catching up.
package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"p2pchain/internal/domain"
	"p2pchain/internal/events"
)

// bootstrapRetryInterval is how often we retry bootstrap peers we are not connected to.
const bootstrapRetryInterval = 5 * time.Second

// Config is the node's networking configuration.
type Config struct {
	ListenPort int
	KeySeed    string   // any string; the peer identity is derived from it, so a node keeps its peer ID
	Bootstrap  []string // multiaddrs of peers to dial at startup
	ChainID    uint64   // topics are namespaced by chain id, so two chains never mix
}

// Handler is what the network needs from the node. Implemented by service.Node.
type Handler interface {
	// OnBlock is called for a gossiped block that passed validation.
	OnBlock(from peer.ID, b domain.Block)
	// OnTx is called for a gossiped transaction that passed validation.
	OnTx(from peer.ID, raw []byte)
	// ValidateBlockForGossip decides whether a block is worth relaying.
	ValidateBlockForGossip(b domain.Block) error
	// ValidateTxForGossip decides whether a transaction is worth relaying.
	ValidateTxForGossip(raw []byte) error
	// Status describes our chain for a peer.
	Status() Status
	// BlocksAfter answers a peer's locator with canonical blocks.
	BlocksAfter(locator []domain.Hash, max int) ([]domain.Block, error)
	// BlockByHash serves one block by hash (used to fetch a missing parent).
	BlockByHash(h domain.Hash) (domain.Block, bool)
	// OnPeerStatus is called when a peer tells us where its chain is.
	OnPeerStatus(p peer.ID, s Status)
}

// Node is the libp2p side of a chain node.
type Node struct {
	host   host.Host
	ps     *pubsub.PubSub
	blocks *pubsub.Topic
	txs    *pubsub.Topic
	h      Handler
	bus    *events.Bus
	cfg    Config

	// blocked simulates a network partition: messages from these peers are
	// ignored and their sync requests are refused, without closing the
	// connection — exactly how a one-way network failure feels.
	blockedMu sync.RWMutex
	blocked   map[peer.ID]bool
}

// identity derives a stable ed25519 key from a seed string. Same seed → same
// peer ID, which is what lets docker-compose hard-code bootstrap addresses.
// Real nodes generate a random key once and keep the file.
func identity(seed string) (libp2pcrypto.PrivKey, error) {
	digest := sha256.Sum256([]byte("p2pchain identity:" + seed))
	return libp2pcrypto.UnmarshalEd25519PrivateKey(ed25519.NewKeyFromSeed(digest[:]))
}

// PeerIDForSeed returns the peer ID a node with this seed will have.
func PeerIDForSeed(seed string) (peer.ID, error) {
	priv, err := identity(seed)
	if err != nil {
		return "", err
	}
	return peer.IDFromPrivateKey(priv)
}

// New starts the host, joins the gossip topics, and dials the bootstrap peers.
func New(ctx context.Context, cfg Config, h Handler, bus *events.Bus) (*Node, error) {
	priv, err := identity(cfg.KeySeed)
	if err != nil {
		return nil, err
	}
	host, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.ListenPort)),
		libp2p.UserAgent("p2pchain/0.1"),
	)
	if err != nil {
		return nil, err
	}

	n := &Node{host: host, h: h, bus: bus, cfg: cfg, blocked: map[peer.ID]bool{}}

	// A message's ID decides what "the same message" means for deduplication.
	// Hashing the payload means the same block gossiped by ten peers is
	// forwarded once, no matter who sent it.
	n.ps, err = pubsub.NewGossipSub(ctx, host, pubsub.WithMessageIdFn(messageID))
	if err != nil {
		host.Close()
		return nil, err
	}
	if n.blocks, err = n.join(blocksTopic(cfg.ChainID), n.validateBlock); err != nil {
		host.Close()
		return nil, err
	}
	if n.txs, err = n.join(txsTopic(cfg.ChainID), n.validateTx); err != nil {
		host.Close()
		return nil, err
	}

	host.SetStreamHandler(syncProtocol, n.handleSyncStream)
	n.watchPeers(ctx)
	go n.readBlocks(ctx)
	go n.readTxs(ctx)
	n.dialBootstrap(ctx)
	return n, nil
}

func (n *Node) Close() error { return n.host.Close() }

func (n *Node) ID() peer.ID { return n.host.ID() }

// Addrs returns this node's full multiaddrs, including the peer ID.
func (n *Node) Addrs() []string {
	var out []string
	for _, a := range n.host.Addrs() {
		out = append(out, fmt.Sprintf("%s/p2p/%s", a, n.host.ID()))
	}
	return out
}

// PeerInfo describes one connected peer.
type PeerInfo struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	Direction string `json:"direction"`
	Blocked   bool   `json:"blocked"`
}

func (n *Node) Peers() []PeerInfo {
	var out []PeerInfo
	for _, p := range n.host.Network().Peers() {
		conns := n.host.Network().ConnsToPeer(p)
		if len(conns) == 0 {
			continue
		}
		out = append(out, PeerInfo{
			ID:        p.String(),
			Address:   conns[0].RemoteMultiaddr().String(),
			Direction: conns[0].Stat().Direction.String(),
			Blocked:   n.isBlocked(p),
		})
	}
	return out
}

// SetBlocked turns the simulated partition on or off for the given peers.
func (n *Node) SetBlocked(ids []peer.ID, blocked bool) {
	n.blockedMu.Lock()
	defer n.blockedMu.Unlock()
	for _, id := range ids {
		if blocked {
			n.blocked[id] = true
		} else {
			delete(n.blocked, id)
		}
	}
}

// BlockAllPeers blocks (or unblocks) every currently connected peer.
func (n *Node) BlockAllPeers(blocked bool) []peer.ID {
	ids := n.host.Network().Peers()
	n.SetBlocked(ids, blocked)
	return ids
}

// ClearBlocked heals every simulated partition.
func (n *Node) ClearBlocked() {
	n.blockedMu.Lock()
	defer n.blockedMu.Unlock()
	n.blocked = map[peer.ID]bool{}
}

func (n *Node) isBlocked(p peer.ID) bool {
	n.blockedMu.RLock()
	defer n.blockedMu.RUnlock()
	return n.blocked[p]
}

// dialBootstrap keeps trying to reach the configured peers.
//
// One attempt is not enough: peers start in any order, a node may still be
// mining its genesis block when we dial, and links go down. So we retry any
// bootstrap peer we are not currently connected to, forever. Real clients do
// the same (and add peer exchange and DNS seeds on top).
func (n *Node) dialBootstrap(ctx context.Context) {
	var peers []peer.AddrInfo
	for _, addr := range n.cfg.Bootstrap {
		info, err := peer.AddrInfoFromString(addr)
		if err != nil {
			log.Printf("p2p: bad bootstrap address %q: %v", addr, err)
			continue
		}
		peers = append(peers, *info)
	}
	if len(peers) == 0 {
		return
	}

	go func() {
		for attempt := 0; ctx.Err() == nil; attempt++ {
			for _, info := range peers {
				if n.host.Network().Connectedness(info.ID) == network.Connected {
					continue
				}
				dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := n.host.Connect(dialCtx, info)
				cancel()
				if err == nil {
					log.Printf("p2p: connected to %s", info.ID.ShortString())
				} else if attempt == 0 {
					log.Printf("p2p: dial %s failed, will retry: %v", info.ID.ShortString(), rootCause(err))
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(bootstrapRetryInterval):
			}
		}
	}()
}

// rootCause trims libp2p's nested dial errors down to the last line.
func rootCause(err error) string {
	lines := strings.Split(strings.TrimSpace(err.Error()), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// watchPeers reports connections and asks every new peer where its chain is.
func (n *Node) watchPeers(ctx context.Context) {
	n.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, c network.Conn) {
			p := c.RemotePeer()
			n.bus.Publish(events.PeerConnected, map[string]any{"peer": p.String()})
			go func() {
				// Give the connection a moment to be usable, then exchange status.
				time.Sleep(200 * time.Millisecond)
				askCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if st, err := n.RequestStatus(askCtx, p); err == nil {
					n.h.OnPeerStatus(p, st)
				}
			}()
		},
		DisconnectedF: func(_ network.Network, c network.Conn) {
			n.bus.Publish(events.PeerDisconnected, map[string]any{"peer": c.RemotePeer().String()})
		},
	})
}
