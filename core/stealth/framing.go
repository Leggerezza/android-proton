// SPDX-License-Identifier: GPL-3.0-or-later
package stealth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// TunSafe-style framing, as used by Proton's Stealth transport
// (WireGuard over TLS). One frame per WireGuard packet:
//
//	byte 0: bits [7:6] = frame type, bits [5:0] = payload length >> 8
//	byte 1: payload length & 0xff
//
// Type 0b00 ("normal") carries a complete WireGuard packet.
// Type 0b10 ("data") drops the 16-byte WireGuard data header, because the
// receiver can rebuild it from the previous packet plus a counter.
const (
	TunSafeHeaderSize = 2
	tunSafeNormalType = byte(0x00)
	tunSafeDataType   = byte(0x80) // 0b10 << 6

	wgDataHeaderSize = 16
	wgDataPrefixSize = 8
	wgDataMsgType    = 4

	MaxFramePayload = 0x3fff
)

// tunSafeState carries the header state shared by both directions.
type tunSafeState struct {
	sendPrefix []byte
	sendCount  uint64
	recvPrefix []byte
	recvCount  uint64
}

func newTunSafeState() *tunSafeState {
	return &tunSafeState{
		sendPrefix: make([]byte, wgDataPrefixSize),
		recvPrefix: make([]byte, wgDataPrefixSize),
	}
}

func (ts *tunSafeState) reset() {
	for i := range ts.sendPrefix {
		ts.sendPrefix[i] = 0
	}
	for i := range ts.recvPrefix {
		ts.recvPrefix[i] = 0
	}
	ts.sendCount = 0
	ts.recvCount = 0
}

func parseTunSafeHeader(hdr []byte) (typ byte, size int) {
	typ = hdr[0] & 0xc0
	size = int(hdr[0]&0x3f)<<8 | int(hdr[1])
	return typ, size
}

// encode converts a WireGuard packet into a framed buffer (dst must be large
// enough: len(pkt)+TunSafeHeaderSize).
func (ts *tunSafeState) encode(pkt []byte, dst []byte) []byte {
	if len(pkt) < wgDataHeaderSize {
		return encodeNormal(pkt, dst)
	}

	prefix := pkt[:wgDataPrefixSize]
	count := binary.LittleEndian.Uint64(pkt[wgDataPrefixSize:wgDataHeaderSize])

	if equalBytes(prefix, ts.sendPrefix) && count == ts.sendCount+1 {
		ts.sendCount++
		return encodeData(pkt, dst)
	}
	if pkt[0] == wgDataMsgType {
		copy(ts.sendPrefix, prefix)
		ts.sendCount = count
	}
	return encodeNormal(pkt, dst)
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func encodeNormal(pkt, dst []byte) []byte {
	size := len(pkt)
	dst[0] = byte(size >> 8)
	dst[1] = byte(size & 0xff)
	copy(dst[TunSafeHeaderSize:], pkt)
	return dst[:TunSafeHeaderSize+size]
}

func encodeData(pkt, dst []byte) []byte {
	size := len(pkt) - wgDataHeaderSize
	dst[0] = tunSafeDataType | byte(size>>8)
	dst[1] = byte(size & 0xff)
	copy(dst[TunSafeHeaderSize:], pkt[wgDataHeaderSize:])
	return dst[:TunSafeHeaderSize+size]
}

// decode rebuilds a WireGuard packet from a frame body. data must hold at
// least size+wgDataHeaderSize bytes.
func (ts *tunSafeState) decode(typ byte, size int, body []byte, data []byte) ([]byte, error) {
	switch typ {
	case tunSafeNormalType:
		if size > len(data) {
			return nil, fmt.Errorf("stealth: frame too large: %d", size)
		}
		pkt := data[:size]
		copy(pkt, body[:size])
		if len(pkt) >= wgDataHeaderSize && pkt[0] == wgDataMsgType {
			copy(ts.recvPrefix, pkt[:wgDataPrefixSize])
			ts.recvCount = binary.LittleEndian.Uint64(pkt[wgDataPrefixSize:wgDataHeaderSize])
		}
		ts.recvCount++
		return pkt, nil

	case tunSafeDataType:
		total := size + wgDataHeaderSize
		if total > len(data) {
			return nil, fmt.Errorf("stealth: frame too large: %d", total)
		}
		pkt := data[:total]
		copy(pkt, ts.recvPrefix)
		binary.LittleEndian.PutUint64(pkt[wgDataPrefixSize:wgDataHeaderSize], ts.recvCount)
		copy(pkt[wgDataHeaderSize:], body[:size])
		ts.recvCount++
		return pkt, nil

	default:
		return nil, fmt.Errorf("stealth: unknown frame type %#x", typ)
	}
}

// writeFrame writes one framed WireGuard packet to w.
func (ts *tunSafeState) writeFrame(w io.Writer, pkt []byte, scratch []byte) error {
	frame := ts.encode(pkt, scratch)
	_, err := w.Write(frame)
	return err
}

// readFrame reads one framed WireGuard packet from r.
func (ts *tunSafeState) readFrame(r io.Reader, hdr []byte, body []byte, data []byte) ([]byte, error) {
	if len(hdr) < TunSafeHeaderSize {
		return nil, errors.New("stealth: header scratch too small")
	}
	if _, err := io.ReadFull(r, hdr[:TunSafeHeaderSize]); err != nil {
		return nil, err
	}
	typ, size := parseTunSafeHeader(hdr[:TunSafeHeaderSize])
	if size == 0 || size > MaxFramePayload {
		return nil, fmt.Errorf("stealth: invalid frame size %d", size)
	}
	if size > len(body) {
		return nil, fmt.Errorf("stealth: frame body too small for %d", size)
	}
	if _, err := io.ReadFull(r, body[:size]); err != nil {
		return nil, err
	}
	return ts.decode(typ, size, body, data)
}
