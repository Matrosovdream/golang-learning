// Command peerid prints the peer ID a node will have for a given NODE_SEED.
//
// Peer IDs are derived from the node's key, and the key is derived from the
// seed, so the IDs are known before the nodes ever start — which is how
// docker-compose can hard-code BOOTSTRAP addresses.
//
//	go run ./cmd/peerid node1 node2 node3
package main

import (
	"fmt"
	"os"

	"p2pchain/internal/p2p"
)

func main() {
	seeds := os.Args[1:]
	if len(seeds) == 0 {
		fmt.Fprintln(os.Stderr, "usage: peerid <seed> [seed...]")
		os.Exit(2)
	}
	for _, seed := range seeds {
		id, err := p2p.PeerIDForSeed(seed)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%-10s %s\n", seed, id)
	}
}
