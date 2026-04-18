//go:build !linux

// packet_other.go — stub for non-Linux platforms.
// Raw socket injection is Linux-only. On other platforms, all bypass
// methods return ErrNotSupported and the proxy runs in relay-only mode.
package bypass

import (
	"fmt"
	"math/rand"
	"net"
)

// PacketParams holds parameters for crafting a raw TCP packet.
type PacketParams struct {
	SrcIP   net.IP
	DstIP   net.IP
	SrcPort uint16
	DstPort uint16
	SeqNum  uint32
	AckNum  uint32
	Flags   uint8
	TTL     uint8
	Window  uint16
	Payload []byte
}

// TCP flag constants
const (
	TCPFlagFIN = 0x01
	TCPFlagSYN = 0x02
	TCPFlagRST = 0x04
	TCPFlagPSH = 0x08
	TCPFlagACK = 0x10
	TCPFlagURG = 0x20
)

// ErrNotSupported is returned on non-Linux platforms.
var ErrNotSupported = fmt.Errorf("raw socket injection not supported on this platform (Linux only)")

// CraftPacket is a stub on non-Linux platforms.
func CraftPacket(p PacketParams) ([]byte, error) {
	return nil, ErrNotSupported
}

// RandomTTL returns a random TTL value.
func RandomTTL() uint8 {
	bases := []uint8{64, 128}
	base := bases[rand.Intn(len(bases))]
	hops := uint8(rand.Intn(8) + 1)
	if hops >= base {
		hops = base - 1
	}
	return base - hops
}

// RandomIPID returns a random IP ID.
func RandomIPID() uint16 {
	return uint16(rand.Uint32())
}

// RawInjector is a stub on non-Linux platforms.
type RawInjector struct{}

// NewRawInjector returns ErrNotSupported on non-Linux platforms.
func NewRawInjector() (*RawInjector, error) {
	return nil, ErrNotSupported
}

// Send is a stub.
func (r *RawInjector) Send(packet []byte, dstIP net.IP, dstPort uint16) error {
	return ErrNotSupported
}

// Close is a stub.
func (r *RawInjector) Close() error { return nil }
