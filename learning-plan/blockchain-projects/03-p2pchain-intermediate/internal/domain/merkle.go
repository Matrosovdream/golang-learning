package domain

import "fmt"

// Merkle tree with domain separation (the RFC 6962 / Certificate Transparency style):
//
//	leaf = SHA-256(0x00 ‖ item)
//	node = SHA-256(0x01 ‖ left ‖ right)
//
// The prefixes make it impossible for an inner node to pass as a leaf. Without
// them, the 64 bytes "left ‖ right" of an inner node are also valid leaf data,
// and someone could "prove" a fake item that is really a subtree.
//
// An odd node at the end of a level is promoted to the next level unchanged.
// Bitcoin instead duplicates it, which makes the lists [a b c] and [a b c c]
// produce the same root (CVE-2012-2459). Promotion doesn't have that problem.

const (
	leafPrefix = 0x00
	nodePrefix = 0x01
)

func hashLeaf(item Hash) Hash {
	var buf [1 + 32]byte
	buf[0] = leafPrefix
	copy(buf[1:], item[:])
	return SHA256(buf[:])
}

func hashNode(left, right Hash) Hash {
	var buf [1 + 32 + 32]byte
	buf[0] = nodePrefix
	copy(buf[1:], left[:])
	copy(buf[33:], right[:])
	return SHA256(buf[:])
}

// MerkleRoot returns the root of a tree over items. An empty list has the zero hash as root.
func MerkleRoot(items []Hash) Hash {
	if len(items) == 0 {
		return Hash{}
	}
	level := leaves(items)
	for len(level) > 1 {
		level = nextLevel(level)
	}
	return level[0]
}

// ProofStep is one sibling on the path from a leaf to the root.
type ProofStep struct {
	Sibling Hash
	Left    bool // true: the sibling is on the LEFT, so hash(sibling ‖ current)
}

// MerkleProof returns the siblings needed to recompute the root from items[index].
// Its length is about log₂(len(items)): 20 steps for a million items.
func MerkleProof(items []Hash, index int) ([]ProofStep, error) {
	if index < 0 || index >= len(items) {
		return nil, fmt.Errorf("merkle proof: index %d out of range [0, %d)", index, len(items))
	}
	var proof []ProofStep
	level := leaves(items)
	for len(level) > 1 {
		switch {
		case index%2 == 1: // we are a right child
			proof = append(proof, ProofStep{Sibling: level[index-1], Left: true})
		case index+1 < len(level): // we are a left child with a right sibling
			proof = append(proof, ProofStep{Sibling: level[index+1], Left: false})
		default: // odd one out: promoted, no sibling at this level
		}
		level = nextLevel(level)
		index /= 2
	}
	return proof, nil
}

// VerifyProof recomputes the root from one item and its proof. It needs only
// 32-byte hashes — not the other items — which is what lets a light client
// check that a transaction is in a block without downloading the block.
func VerifyProof(item Hash, proof []ProofStep, root Hash) bool {
	h := hashLeaf(item)
	for _, step := range proof {
		if step.Left {
			h = hashNode(step.Sibling, h)
		} else {
			h = hashNode(h, step.Sibling)
		}
	}
	return h == root
}

func leaves(items []Hash) []Hash {
	level := make([]Hash, len(items))
	for i, item := range items {
		level[i] = hashLeaf(item)
	}
	return level
}

func nextLevel(level []Hash) []Hash {
	next := make([]Hash, 0, (len(level)+1)/2)
	for i := 0; i < len(level); i += 2 {
		if i+1 == len(level) {
			next = append(next, level[i]) // promote the odd node
			break
		}
		next = append(next, hashNode(level[i], level[i+1]))
	}
	return next
}
