package tls

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

type Builder struct {
	profile *Profile
}

func NewBuilder(profileName string) *Builder {
	return &Builder{profile: GetProfile(profileName)}
}

func NewBuilderWithProfile(p *Profile) *Builder {
	return &Builder{profile: p}
}

func (b *Builder) Build(sni string) ([]byte, error) {
	p := b.profile

	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("clienthello: rand: %w", err)
	}
	sessionID := make([]byte, 32)
	if _, err := rand.Read(sessionID); err != nil {
		return nil, fmt.Errorf("clienthello: rand session: %w", err)
	}
	keySharePub := make([]byte, 32)
	if _, err := rand.Read(keySharePub); err != nil {
		return nil, fmt.Errorf("clienthello: rand keyshare: %w", err)
	}

	csBuf := newBuf()
	if p.UseGREASE {
		csBuf.writeUint16(randomGREASE())
	}
	for _, cs := range p.CipherSuites {
		csBuf.writeUint16(cs)
	}
	csBuf.writeUint16(0x00FF)

	extMap := b.buildExtensions(sni, sessionID, keySharePub, p)

	extBuf := newBuf()
	if p.UseGREASE {
		g := randomGREASE()
		extBuf.writeExtension(g, []byte{0x00})
	}
	for _, extType := range p.ExtensionOrder {
		if data, ok := extMap[extType]; ok {
			extBuf.writeExtension(extType, data)
		}
	}

	currentLen := extBuf.len() + 2
	helloBodyLen := 2 + 32 + 1 + len(sessionID) + 2 + csBuf.len() + 1 + 1 + 2 + currentLen
	jitter := fastRandN(33) - 16
	target := 512 + jitter
	padNeeded := target - helloBodyLen - 4
	if padNeeded > 0 {
		extBuf.writeExtension(extPadding, make([]byte, padNeeded))
	}

	hello := newBuf()
	hello.writeUint16(0x0303)
	hello.writeBytes(random)
	hello.writeUint8(byte(len(sessionID)))
	hello.writeBytes(sessionID)
	hello.writeUint16(uint16(csBuf.len()))
	hello.writeBytes(csBuf.bytes())
	hello.writeUint8(1)
	hello.writeUint8(0x00)
	hello.writeUint16(uint16(extBuf.len()))
	hello.writeBytes(extBuf.bytes())

	hs := newBuf()
	hs.writeUint8(0x01)
	hs.writeUint24(uint32(hello.len()))
	hs.writeBytes(hello.bytes())

	rec := newBuf()
	rec.writeUint8(0x16)
	rec.writeUint16(0x0301)
	rec.writeUint16(uint16(hs.len()))
	rec.writeBytes(hs.bytes())

	return rec.bytes(), nil
}

func (b *Builder) buildExtensions(sni string, sessionID, keySharePub []byte, p *Profile) map[uint16][]byte {
	m := make(map[uint16][]byte, 20)

	sniBytes := []byte(sni)
	sniBuf := newBuf()
	sniBuf.writeUint8(0x00)
	sniBuf.writeUint16(uint16(len(sniBytes)))
	sniBuf.writeBytes(sniBytes)
	sniList := newBuf()
	sniList.writeUint16(uint16(sniBuf.len()))
	sniList.writeBytes(sniBuf.bytes())
	m[extSNI] = sniList.bytes()

	m[extExtendedMasterSec] = []byte{}
	m[extRenegotiationInfo] = []byte{0x00}

	grpBuf := newBuf()
	for _, g := range p.SupportedGroups {
		grpBuf.writeUint16(g)
	}
	sgBuf := newBuf()
	sgBuf.writeUint16(uint16(grpBuf.len()))
	sgBuf.writeBytes(grpBuf.bytes())
	m[extSupportedGroups] = sgBuf.bytes()

	m[extECPointFormats] = []byte{0x01, 0x00}
	m[extSessionTicket] = []byte{}

	alpnProtos := newBuf()
	for _, proto := range p.ALPNProtos {
		alpnProtos.writeUint8(byte(len(proto)))
		alpnProtos.writeBytes([]byte(proto))
	}
	alpnBuf := newBuf()
	alpnBuf.writeUint16(uint16(alpnProtos.len()))
	alpnBuf.writeBytes(alpnProtos.bytes())
	m[extALPN] = alpnBuf.bytes()

	m[extStatusRequest] = []byte{0x01, 0x00, 0x00, 0x00, 0x00}
	m[extSignedCertTS] = []byte{}

	ksEntry := newBuf()
	ksEntry.writeUint16(0x001D)
	ksEntry.writeUint16(32)
	ksEntry.writeBytes(keySharePub)
	ksBuf := newBuf()
	ksBuf.writeUint16(uint16(ksEntry.len()))
	ksBuf.writeBytes(ksEntry.bytes())
	m[extKeyShare] = ksBuf.bytes()

	svBuf := newBuf()
	svBuf.writeUint8(byte(len(p.TLSVersions) * 2))
	for _, v := range p.TLSVersions {
		svBuf.writeUint16(v)
	}
	m[extSupportedVersions] = svBuf.bytes()

	if p.CompressCert != nil {
		m[extCompressCert] = p.CompressCert
	}

	pskBuf := newBuf()
	pskBuf.writeUint8(byte(len(p.PSKModes)))
	pskBuf.writeBytes(p.PSKModes)
	m[extPSKKeyExchModes] = pskBuf.bytes()

	saBuf := newBuf()
	for _, sa := range p.SigAlgs {
		saBuf.writeUint16(sa)
	}
	saWrap := newBuf()
	saWrap.writeUint16(uint16(saBuf.len()))
	saWrap.writeBytes(saBuf.bytes())
	m[extSigAlgs] = saWrap.bytes()

	return m
}

type buf struct {
	data []byte
}

func newBuf() *buf { return &buf{data: make([]byte, 0, 256)} }

func (b *buf) writeUint8(v byte)   { b.data = append(b.data, v) }
func (b *buf) writeBytes(v []byte) { b.data = append(b.data, v...) }
func (b *buf) len() int            { return len(b.data) }
func (b *buf) bytes() []byte       { return b.data }

func (b *buf) writeUint16(v uint16) {
	b.data = append(b.data, byte(v>>8), byte(v))
}

func (b *buf) writeUint24(v uint32) {
	b.data = append(b.data, byte(v>>16), byte(v>>8), byte(v))
}

func (b *buf) writeExtension(extType uint16, data []byte) {
	b.writeUint16(extType)
	b.writeUint16(uint16(len(data)))
	b.writeBytes(data)
}

func JA3String(record []byte) string {
	if len(record) < 9 {
		return ""
	}
	data := record[9:]
	if len(data) < 34 {
		return ""
	}
	data = data[34:]
	if len(data) < 1 {
		return ""
	}
	sessLen := int(data[0])
	data = data[1+sessLen:]
	if len(data) < 2 {
		return ""
	}
	csLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if len(data) < csLen {
		return ""
	}
	csData := data[:csLen]
	data = data[csLen:]
	if len(data) < 1 {
		return ""
	}
	compLen := int(data[0])
	data = data[1+compLen:]
	if len(data) < 2 {
		return ""
	}
	extLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if len(data) < extLen {
		return ""
	}
	extData := data[:extLen]

	var ciphers []uint16
	for i := 0; i+1 < len(csData); i += 2 {
		cs := binary.BigEndian.Uint16(csData[i:])
		if !isGREASE(cs) {
			ciphers = append(ciphers, cs)
		}
	}

	var extTypes, groups []uint16
	var ecPoints []byte
	for len(extData) >= 4 {
		et := binary.BigEndian.Uint16(extData[:2])
		el := int(binary.BigEndian.Uint16(extData[2:4]))
		extData = extData[4:]
		if len(extData) < el {
			break
		}
		eBody := extData[:el]
		extData = extData[el:]
		if isGREASE(et) {
			continue
		}
		extTypes = append(extTypes, et)
		if et == extSupportedGroups && len(eBody) >= 2 {
			gLen := int(binary.BigEndian.Uint16(eBody[:2]))
			gData := eBody[2:]
			for i := 0; i+1 < gLen && i+1 < len(gData); i += 2 {
				g := binary.BigEndian.Uint16(gData[i:])
				if !isGREASE(g) {
					groups = append(groups, g)
				}
			}
		}
		if et == extECPointFormats && len(eBody) >= 1 {
			fmtLen := int(eBody[0])
			if len(eBody) >= 1+fmtLen {
				ecPoints = eBody[1 : 1+fmtLen]
			}
		}
	}

	result := "771,"
	result += joinUint16(ciphers) + ","
	result += joinUint16(extTypes) + ","
	result += joinUint16(groups) + ","
	for i, b := range ecPoints {
		if i > 0 {
			result += "-"
		}
		result += fmt.Sprintf("%d", b)
	}
	return result
}

func isGREASE(v uint16) bool {
	return v&0x0F0F == 0x0A0A && v>>8 == v&0xFF
}

func joinUint16(vals []uint16) string {
	s := ""
	for i, v := range vals {
		if i > 0 {
			s += "-"
		}
		s += fmt.Sprintf("%d", v)
	}
	return s
}
