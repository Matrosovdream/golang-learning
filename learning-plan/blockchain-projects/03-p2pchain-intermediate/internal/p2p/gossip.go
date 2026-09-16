package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"

	"p2pchain/internal/domain"
)

func blocksTopic(chainID uint64) string { return fmt.Sprintf("p2pchain/%d/blocks/1", chainID) }
func txsTopic(chainID uint64) string    { return fmt.Sprintf("p2pchain/%d/txs/1", chainID) }

// messageID identifies a gossip message by its content hash, so the same block
// relayed by many peers is processed and forwarded exactly once.
func messageID(m *pubsubpb.Message) string {
	sum := sha256.Sum256(m.Data)
	return hex.EncodeToString(sum[:])
}

func (n *Node) join(topic string, validator pubsub.ValidatorEx) (*pubsub.Topic, error) {
	// The validator runs BEFORE the message is forwarded to other peers. This
	// is the network's immune system: an invalid block stops at the first
	// honest node instead of being amplified across the whole network.
	if err := n.ps.RegisterTopicValidator(topic, validator); err != nil {
		return nil, err
	}
	return n.ps.Join(topic)
}

// validateBlock decides the fate of a gossiped block:
//
//	Reject — provably invalid (bad proof-of-work, wrong shape). GossipSub
//	         remembers who sent it and lowers that peer's score.
//	Ignore — not forwarded, but no blame (a partitioned peer, a duplicate).
//	Accept — forward to our peers and hand it to the node.
func (n *Node) validateBlock(ctx context.Context, from peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	if n.isBlocked(msg.ReceivedFrom) {
		return pubsub.ValidationIgnore // simulated partition
	}
	b, err := domain.DecodeBlock(msg.Data)
	if err != nil {
		return pubsub.ValidationReject
	}
	if err := n.h.ValidateBlockForGossip(b); err != nil {
		log.Printf("p2p: rejecting block %s from %s: %v", b.Hash.Hex()[:12], msg.ReceivedFrom.ShortString(), err)
		return pubsub.ValidationReject
	}
	msg.ValidatorData = b // decoded once, reused by the reader
	return pubsub.ValidationAccept
}

func (n *Node) validateTx(ctx context.Context, from peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	if n.isBlocked(msg.ReceivedFrom) {
		return pubsub.ValidationIgnore
	}
	if err := n.h.ValidateTxForGossip(msg.Data); err != nil {
		return pubsub.ValidationReject
	}
	return pubsub.ValidationAccept
}

func (n *Node) readBlocks(ctx context.Context) {
	sub, err := n.blocks.Subscribe()
	if err != nil {
		log.Printf("p2p: subscribe blocks: %v", err)
		return
	}
	defer sub.Cancel()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return // context cancelled or subscription closed
		}
		if msg.ReceivedFrom == n.host.ID() {
			continue // our own broadcast comes back to us
		}
		b, ok := msg.ValidatorData.(domain.Block)
		if !ok {
			continue
		}
		n.h.OnBlock(msg.ReceivedFrom, b)
	}
}

func (n *Node) readTxs(ctx context.Context) {
	sub, err := n.txs.Subscribe()
	if err != nil {
		log.Printf("p2p: subscribe txs: %v", err)
		return
	}
	defer sub.Cancel()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == n.host.ID() {
			continue
		}
		n.h.OnTx(msg.ReceivedFrom, msg.Data)
	}
}

// BroadcastBlock publishes a block we accepted (usually one we mined).
func (n *Node) BroadcastBlock(ctx context.Context, b domain.Block) error {
	return n.blocks.Publish(ctx, domain.EncodeBlock(b))
}

// BroadcastTx publishes a raw transaction we accepted into our mempool.
func (n *Node) BroadcastTx(ctx context.Context, raw []byte) error {
	return n.txs.Publish(ctx, raw)
}

// GossipPeers counts peers we share the block topic with.
func (n *Node) GossipPeers() int { return len(n.ps.ListPeers(blocksTopic(n.cfg.ChainID))) }
