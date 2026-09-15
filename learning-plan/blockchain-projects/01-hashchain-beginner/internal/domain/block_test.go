package domain

import (
	"encoding/binary"
	"testing"
)

func sampleHeader() Header {
	return Header{
		Height:     7,
		Timestamp:  1_767_225_600_000, // 2026-01-01T00:00:00Z
		PrevHash:   HashData([]byte("parent")),
		DataHash:   HashData([]byte("alice pays bob 5")),
		Difficulty: 16,
		Nonce:      42,
	}
}

func TestEncodeIsFixedSizeWithNonceLast(t *testing.T) {
	h := sampleHeader()
	buf := h.Encode()
	if len(buf) != HeaderSize {
		t.Fatalf("encoded %d bytes, want %d", len(buf), HeaderSize)
	}
	if got := binary.BigEndian.Uint64(buf[HeaderSize-8:]); got != h.Nonce {
		t.Fatalf("last 8 bytes decode to %d, want nonce %d", got, h.Nonce)
	}
}

// A golden value pins the encoding: if anyone reorders fields or changes a
// width, every hash on the chain changes, and this test says so loudly.
func TestComputeHashGolden(t *testing.T) {
	const want = "84de076885e47b15db818cce4747281b98b61f5acd6848ce5b9a955b34f4fa44"
	if got := sampleHeader().ComputeHash().String(); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
}

func TestHashChangesWhenAnyFieldChanges(t *testing.T) {
	base := sampleHeader().ComputeHash()
	mutations := map[string]func(*Header){
		"height":     func(h *Header) { h.Height++ },
		"timestamp":  func(h *Header) { h.Timestamp++ },
		"prev hash":  func(h *Header) { h.PrevHash[31] ^= 1 },
		"data hash":  func(h *Header) { h.DataHash[0] ^= 1 },
		"difficulty": func(h *Header) { h.Difficulty++ },
		"nonce":      func(h *Header) { h.Nonce++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			h := sampleHeader()
			mutate(&h)
			if h.ComputeHash() == base {
				t.Fatalf("changing %s did not change the hash", name)
			}
		})
	}
}
