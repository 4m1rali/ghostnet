//go:build linux

// packet_linux.go — raw TCP/IP packet crafting for Linux (AF_PACKET / raw sockets).
// Provides manual IP+TCP header construction, checksum computation,
// and injection via raw sockets.
package bypass

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"syscall"
)

// ─── IP Header ────────────────────────────────────────────────────────────────

// IPHeader represents an IPv4 header (20 bytes, no options).
type IPHeader struct {
	VersionIHL    uint8
	TOS           uint8
	TotalLength   uint16
	ID            uint16
	FlagsFragment uint16
	TTL           uint8
	Protocol      uint8
	Checksum      uint16
	SrcIP         [4]byte
	DstIP         [4]byte
}

func (h *IPHeader) marshal() []byte {
	b := make([]byte, 20)
	b[0] = h.VersionIHL
	b[1] = h.TOS
	binary.BigEndian.PutUint16(b[2:], h.TotalLength)
	binary.BigEndian.PutUint16(b[4:], h.ID)
	binary.BigEndian.PutUint16(b[6:], h.FlagsFragment)
	b[8] = h.TTL
	b[9] = h.Protocol
	copy(b[12:16], h.SrcIP[:])
	copy(b[16:20], h.DstIP[:])
	cs := ipChecksum(b)
	binary.BigEndian.PutUint16(b[10:], cs)
	return b
}

// ─── TCP Header ───────────────────────────────────────────────────────────────

// TCPHeader represents a TCP header (20 bytes, no options).
type TCPHeader struct {
	SrcPort    uint16
	DstPort    uint16
	SeqNum     uint32
	AckNum     uint32
	DataOffset uint8
	Flags      uint8
	Window     uint16
	Checksum   uint16
	Urgent     uint16
}

const (
	TCPFlagFIN = 0x01
	TCPFlagSYN = 0x02
	TCPFlagRST = 0x04
	TCPFlagPSH = 0x08
	TCPFlagACK = 0x10
	TCPFlagURG = 0x20
)

func (h *TCPHeader) marshal(srcIP, dstIP [4]byte, payload []byte) []byte {
	hdrLen := 20
	b := make([]byte, hdrLen+len(payload))
	binary.BigEndian.PutUint16(b[0:], h.SrcPort)
	binary.BigEndian.PutUint16(b[2:], h.DstPort)
	binary.BigEndian.PutUint32(b[4:], h.SeqNum)
	binary.BigEndian.PutUint32(b[8:], h.AckNum)
	b[12] = h.DataOffset
	b[13] = h.Flags
	binary.BigEndian.PutUint16(b[14:], h.Window)
	binary.BigEndian.PutUint16(b[18:], h.Urgent)
	copy(b[hdrLen:], payload)
	cs := tcpChecksum(srcIP, dstIP, b)
	binary.BigEndian.PutUint16(b[16:], cs)
	return b
}

// ─── Checksum ─────────────────────────────────────────────────────────────────

func ipChecksum(b []byte) uint16 {
	return internetChecksum(b)
}

func tcpChecksum(srcIP, dstIP [4]byte, tcpSegment []byte) uint16 {
	pseudo := make([]byte, 12+len(tcpSegment))
	copy(pseudo[0:4], srcIP[:])
	copy(pseudo[4:8], dstIP[:])
	pseudo[8] = 0
	pseudo[9] = syscall.IPPROTO_TCP
	binary.BigEndian.PutUint16(pseudo[10:], uint16(len(tcpSegment)))
	copy(pseudo[12:], tcpSegment)
	return internetChecksum(pseudo)
}

func internetChecksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	if len(data)%2 != 0 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return ^uint16(sum)
}

// ─── Packet Builder ───────────────────────────────────────────────────────────

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

// CraftPacket builds a complete IPv4+TCP packet ready for raw socket injection.
func CraftPacket(p PacketParams) ([]byte, error) {
	src4 := p.SrcIP.To4()
	dst4 := p.DstIP.To4()
	if src4 == nil || dst4 == nil {
		return nil, fmt.Errorf("packet: IPv6 not supported for raw crafting")
	}

	var srcArr, dstArr [4]byte
	copy(srcArr[:], src4)
	copy(dstArr[:], dst4)

	tcpHdr := &TCPHeader{
		SrcPort:    p.SrcPort,
		DstPort:    p.DstPort,
		SeqNum:     p.SeqNum,
		AckNum:     p.AckNum,
		DataOffset: 0x50,
		Flags:      p.Flags,
		Window:     p.Window,
	}
	if tcpHdr.Window == 0 {
		tcpHdr.Window = 65535
	}
	tcpBytes := tcpHdr.marshal(srcArr, dstArr, p.Payload)

	ttl := p.TTL
	if ttl == 0 {
		ttl = RandomTTL()
	}

	ipHdr := &IPHeader{
		VersionIHL:    0x45,
		TOS:           0,
		TotalLength:   uint16(20 + len(tcpBytes)),
		ID:            RandomIPID(),
		FlagsFragment: 0x4000,
		TTL:           ttl,
		Protocol:      syscall.IPPROTO_TCP,
	}
	copy(ipHdr.SrcIP[:], srcArr[:])
	copy(ipHdr.DstIP[:], dstArr[:])

	ipBytes := ipHdr.marshal()
	return append(ipBytes, tcpBytes...), nil
}

// ─── TTL / IP ID helpers ──────────────────────────────────────────────────────

var commonTTLs = []uint8{64, 128}

// RandomTTL returns a TTL that mimics a real host at network distance.
func RandomTTL() uint8 {
	base := commonTTLs[rand.Intn(len(commonTTLs))]
	hops := uint8(rand.Intn(8) + 1)
	if hops >= base {
		hops = base - 1
	}
	return base - hops
}

// RandomIPID returns a random 16-bit IP identification field.
func RandomIPID() uint16 {
	return uint16(rand.Uint32())
}

// ─── Raw Socket Injector ──────────────────────────────────────────────────────

// RawInjector sends raw IP packets via a SOCK_RAW socket.
// Requires CAP_NET_RAW on Linux.
type RawInjector struct {
	fd int
}

// NewRawInjector opens a raw socket for TCP injection.
func NewRawInjector() (*RawInjector, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return nil, fmt.Errorf("raw socket: %w (need CAP_NET_RAW or root)", err)
	}
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("raw socket IP_HDRINCL: %w", err)
	}
	return &RawInjector{fd: fd}, nil
}

// Send injects a pre-crafted raw packet.
func (r *RawInjector) Send(packet []byte, dstIP net.IP, dstPort uint16) error {
	dst4 := dstIP.To4()
	if dst4 == nil {
		return fmt.Errorf("raw inject: IPv6 not supported")
	}
	addr := &syscall.SockaddrInet4{
		Port: int(dstPort),
	}
	copy(addr.Addr[:], dst4)
	return syscall.Sendto(r.fd, packet, 0, addr)
}

// Close releases the raw socket.
func (r *RawInjector) Close() error {
	return syscall.Close(r.fd)
}
