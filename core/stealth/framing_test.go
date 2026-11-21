// SPDX-License-Identifier: GPL-3.0-or-later
package stealth

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func wgDataPacket(counter uint64, payload int) []byte {
	pkt := make([]byte, wgDataHeaderSize+payload)
	pkt[0] = wgDataMsgType
	copy(pkt[1:5], []byte{0xde, 0xad, 0xbe, 0xef})
	binary.LittleEndian.PutUint64(pkt[wgDataPrefixSize:wgDataHeaderSize], counter)
	for i := wgDataHeaderSize; i < len(pkt); i++ {
		pkt[i] = byte(i)
	}
	return pkt
}

// TestFrameRoundTrip checks that a stream of WireGuard packets survives
// encoding and decoding unchanged, including the compressed "data" frames.
func TestFrameRoundTrip(t *testing.T) {
	var stream bytes.Buffer
	enc := newTunSafeState()
	scratch := make([]byte, 70000)

	first := wgDataPacket(1, 100)
	if err := enc.writeFrame(&stream, first, scratch); err != nil {
		t.Fatal(err)
	}
	// Consecutive counter => compressed data frame (header stripped).
	second := wgDataPacket(2, 50)
	if err := enc.writeFrame(&stream, second, scratch); err != nil {
		t.Fatal(err)
	}
	if got := stream.Len(); got != TunSafeHeaderSize+len(first)+TunSafeHeaderSize+50 {
		t.Fatalf("unexpected framed size %d: the second packet was not compressed", got)
	}

	dec := newTunSafeState()
	var hdr [TunSafeHeaderSize]byte
	body := make([]byte, MaxFramePayload)
	data := make([]byte, MaxFramePayload+wgDataHeaderSize+1)

	got1, err := dec.readFrame(&stream, hdr[:], body, data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got1, first) {
		t.Fatalf("first packet mismatch")
	}
	got2, err := dec.readFrame(&stream, hdr[:], body, data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, second) {
		t.Fatalf("second packet mismatch: got %x want %x", got2[:16], second[:16])
	}
}

// TestHandshakeFrameIsNormal makes sure short (handshake) packets are sent
// verbatim, never compressed.
func TestHandshakeFrameIsNormal(t *testing.T) {
	hs := make([]byte, 148)
	hs[0] = 1
	st := newTunSafeState()
	scratch := make([]byte, 200)
	out := st.encode(hs, scratch)
	if len(out) != len(hs)+TunSafeHeaderSize {
		t.Fatalf("handshake frame length %d", len(out))
	}
	if out[0] != 0 || out[1] != 148 {
		t.Fatalf("bad header %x %x", out[0], out[1])
	}
	if !bytes.Equal(out[TunSafeHeaderSize:], hs) {
		t.Fatal("handshake payload altered")
	}
}

// TestDecodeRejectsGarbage ensures a corrupt frame is reported, not panicked.
func TestDecodeRejectsGarbage(t *testing.T) {
	st := newTunSafeState()
	var hdr [TunSafeHeaderSize]byte
	body := make([]byte, 16)
	data := make([]byte, 64)
	if _, err := st.readFrame(bytes.NewReader([]byte{0x40, 0x00}), hdr[:], body, data); err == nil {
		t.Fatal("expected error for zero-length frame")
	}
}
