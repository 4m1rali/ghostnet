package tls

import (
	"encoding/binary"
)

func ExtractSNI(data []byte) (sni string, done bool) {
	if len(data) < 5 {
		return "", false
	}
	if data[0] != 0x16 {
		return "", true
	}
	recLen := int(binary.BigEndian.Uint16(data[3:5]))
	if len(data) < 5+recLen {
		return "", false
	}
	payload := data[5 : 5+recLen]
	if len(payload) < 4 || payload[0] != 0x01 {
		return "", true
	}
	hsLen := int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if len(payload) < 4+hsLen {
		return "", false
	}
	hello := payload[4 : 4+hsLen]
	if len(hello) < 34 {
		return "", true
	}
	pos := 34
	if pos >= len(hello) {
		return "", true
	}
	sessLen := int(hello[pos])
	pos += 1 + sessLen
	if pos+2 > len(hello) {
		return "", true
	}
	csLen := int(binary.BigEndian.Uint16(hello[pos : pos+2]))
	pos += 2 + csLen
	if pos >= len(hello) {
		return "", true
	}
	compLen := int(hello[pos])
	pos += 1 + compLen
	if pos+2 > len(hello) {
		return "", true
	}
	extTotalLen := int(binary.BigEndian.Uint16(hello[pos : pos+2]))
	pos += 2
	if pos+extTotalLen > len(hello) {
		return "", true
	}
	extData := hello[pos : pos+extTotalLen]
	for len(extData) >= 4 {
		extType := binary.BigEndian.Uint16(extData[:2])
		extLen := int(binary.BigEndian.Uint16(extData[2:4]))
		extData = extData[4:]
		if len(extData) < extLen {
			break
		}
		body := extData[:extLen]
		extData = extData[extLen:]
		if extType != 0x0000 {
			continue
		}
		if len(body) < 5 || body[2] != 0x00 {
			return "", true
		}
		nameLen := int(binary.BigEndian.Uint16(body[3:5]))
		if len(body) < 5+nameLen {
			return "", true
		}
		return string(body[5 : 5+nameLen]), true
	}
	return "", true
}

type SNIAccumulator struct {
	buf     []byte
	maxSize int
}

func NewSNIAccumulator(maxSize int) *SNIAccumulator {
	if maxSize <= 0 {
		maxSize = 16384
	}
	return &SNIAccumulator{buf: make([]byte, 0, 512), maxSize: maxSize}
}

func (a *SNIAccumulator) Feed(chunk []byte) (sni string, done bool) {
	a.buf = append(a.buf, chunk...)
	if len(a.buf) > a.maxSize {
		return "", true
	}
	return ExtractSNI(a.buf)
}

func (a *SNIAccumulator) Bytes() []byte {
	return a.buf
}
