package domain

import (
	"fmt"
	"testing"
)

func items(n int) []Hash {
	out := make([]Hash, n)
	for i := range out {
		out[i] = SHA256(fmt.Appendf(nil, "tx %d", i))
	}
	return out
}

func TestEveryLeafHasAWorkingProof(t *testing.T) {
	for n := 1; n <= 17; n++ { // covers odd counts at several levels
		list := items(n)
		root := MerkleRoot(list)
		for i := range list {
			proof, err := MerkleProof(list, i)
			if err != nil {
				t.Fatal(err)
			}
			if !VerifyProof(list[i], proof, root) {
				t.Fatalf("n=%d: proof for leaf %d does not verify", n, i)
			}
		}
	}
}

func TestProofLengthIsLogarithmic(t *testing.T) {
	list := items(1000)
	proof, _ := MerkleProof(list, 777)
	if len(proof) > 10 { // ⌈log₂ 1000⌉ = 10
		t.Fatalf("proof has %d steps for 1000 leaves", len(proof))
	}
}

func TestTamperedProofsFail(t *testing.T) {
	list := items(8)
	root := MerkleRoot(list)
	proof, _ := MerkleProof(list, 3)

	if VerifyProof(list[4], proof, root) {
		t.Error("proof for leaf 3 verified leaf 4")
	}
	bad := append([]ProofStep(nil), proof...)
	bad[1].Sibling[0] ^= 1
	if VerifyProof(list[3], bad, root) {
		t.Error("tampered sibling verified")
	}
	flipped := append([]ProofStep(nil), proof...)
	flipped[0].Left = !flipped[0].Left
	if VerifyProof(list[3], flipped, root) {
		t.Error("wrong sibling position verified")
	}
}

func TestOrderMatters(t *testing.T) {
	list := items(4)
	swapped := []Hash{list[1], list[0], list[2], list[3]}
	if MerkleRoot(list) == MerkleRoot(swapped) {
		t.Fatal("reordering transactions kept the same root")
	}
}

// Bitcoin duplicates the last node of an odd level, so [a b c] and [a b c c]
// share a root (CVE-2012-2459). Promoting the odd node avoids that.
func TestDuplicatedLastItemChangesTheRoot(t *testing.T) {
	list := items(3)
	withDuplicate := append(append([]Hash(nil), list...), list[2])
	if MerkleRoot(list) == MerkleRoot(withDuplicate) {
		t.Fatal("[a b c] and [a b c c] have the same root")
	}
}

// Without leaf/node prefixes, an inner node's hash could be presented as an
// "item" with a shorter proof. With them, it can't.
func TestInnerNodeCannotPoseAsALeaf(t *testing.T) {
	list := items(4)
	root := MerkleRoot(list)
	l := leaves(list)
	inner := hashNode(l[0], l[1])                                     // the parent of items 0 and 1
	fake := []ProofStep{{Sibling: hashNode(l[2], l[3]), Left: false}} // its sibling subtree
	if VerifyProof(inner, fake, root) {
		t.Fatal("an inner node was accepted as a leaf")
	}
}

func TestEmptyRootIsZero(t *testing.T) {
	if !MerkleRoot(nil).IsZero() {
		t.Fatal("empty list should have the zero root")
	}
}
