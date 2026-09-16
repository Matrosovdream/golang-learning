package domain

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestKnownKeysGiveKnownAddresses(t *testing.T) {
	// Private key 1 is famous: every Ethereum tool derives this address from it.
	_, addr := testKey(t, 1)
	if addr.Hex() != "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf" {
		t.Fatalf("address = %s", addr.Hex())
	}
}

func TestSenderIsRecoveredFromTheSignature(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	tx := signed(t, alice, Tx{Nonce: 0, To: bob, Amount: 5, Fee: 1})

	sender, err := tx.Sender()
	if err != nil {
		t.Fatal(err)
	}
	if sender != aliceAddr {
		t.Fatalf("sender = %s, want %s", sender.Hex(), aliceAddr.Hex())
	}
}

// Changing any signed field after signing doesn't produce an error — it
// produces a DIFFERENT sender: some random address that has no coins.
func TestEditingASignedTxChangesTheSender(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	_, mallory := testKey(t, 3)
	base := signed(t, alice, Tx{Nonce: 4, To: bob, Amount: 5, Fee: 1})

	edits := map[string]func(*Tx){
		"amount":   func(tx *Tx) { tx.Amount = 5000 },
		"to":       func(tx *Tx) { tx.To = mallory },
		"fee":      func(tx *Tx) { tx.Fee = 0 },
		"nonce":    func(tx *Tx) { tx.Nonce = 5 },
		"chain id": func(tx *Tx) { tx.ChainID = 1 },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			tx := base
			edit(&tx)
			sender, err := tx.Sender()
			if err == nil && sender == aliceAddr {
				t.Fatalf("editing %s still recovers alice", name)
			}
		})
	}
}

func TestWalletStyleVIsAccepted(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	tx := Tx{ChainID: 1337, Amount: 1, Fee: 1}
	h := tx.SigningHash()
	sig, _ := crypto.Sign(h[:], alice)
	sig[64] += 27 // how MetaMask & co. return it

	withSig, err := tx.WithSignature(sig)
	if err != nil {
		t.Fatal(err)
	}
	if sender, err := withSig.Sender(); err != nil || sender != aliceAddr {
		t.Fatalf("sender = %s, %v", sender.Hex(), err)
	}
}

// Every ECDSA signature (r, s) has a twin (r, n−s) that verifies too. Accepting
// both would let anyone change a tx's bytes — and so its hash — without the key.
func TestHighSTwinSignatureIsRejected(t *testing.T) {
	alice, _ := testKey(t, 1)
	tx := signed(t, alice, Tx{Amount: 1, Fee: 1})

	n := crypto.S256().Params().N
	s := new(big.Int).SetBytes(tx.Sig[32:64])
	twin := tx
	new(big.Int).Sub(n, s).FillBytes(twin.Sig[32:64])
	twin.Sig[64] ^= 1 // the twin recovers with the other parity

	if _, err := twin.Sender(); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("high-s twin: err = %v, want ErrInvalidSignature", err)
	}
}

func TestUnsignedTxIsInvalid(t *testing.T) {
	if _, err := (Tx{Amount: 1}).Sender(); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestRawEncodingRoundTripAndStableHash(t *testing.T) {
	alice, _ := testKey(t, 1)
	_, bob := testKey(t, 2)
	tx := signed(t, alice, Tx{Nonce: 7, To: bob, Amount: 123456, Fee: 3})

	decoded, err := DecodeTx(tx.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if decoded != tx || decoded.Hash() != tx.Hash() {
		t.Fatal("decode(encode(tx)) differs from tx")
	}
	if _, err := DecodeTx(append(tx.Encode(), 0x00)); err == nil {
		t.Fatal("trailing bytes were accepted")
	}
}

// Signatures from go-ethereum are deterministic (RFC 6979), so the whole signed
// tx — and its hash — is reproducible. This pins the encoding and signing.
func TestTxHashGolden(t *testing.T) {
	alice, _ := testKey(t, 1)
	_, bob := testKey(t, 2)
	tx := signed(t, alice, Tx{Nonce: 0, To: bob, Amount: 5, Fee: 1})
	const want = "0x8e3f6831c66d2b19d998cd7d60c7498bd6af0713c93dddddcb9e31134527a402"
	if got := tx.Hash().Hex(); got != want {
		t.Fatalf("tx hash = %s, want %s", got, want)
	}
}

func TestCostOverflow(t *testing.T) {
	if _, ok := (Tx{Amount: ^uint64(0), Fee: 1}).Cost(); ok {
		t.Fatal("max uint64 + 1 did not report overflow")
	}
}

func TestParseAddressChecksum(t *testing.T) {
	good := "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"
	for _, s := range []string{good, "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf", "7E5F4552091A69125D5DFCB7B8C2659029395BDF"} {
		if _, err := ParseAddress(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	typo := "0x7E5F4552091A69125d5DfCb7b8C2659029395BDf" // one letter's case flipped
	if _, err := ParseAddress(typo); err == nil {
		t.Error("bad checksum accepted")
	}
	if _, err := ParseAddress("0x1234"); err == nil {
		t.Error("short address accepted")
	}
}
