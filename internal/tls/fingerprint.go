package tls

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	mrand "math/rand"
	"sync"
)

var greaseValues = []uint16{
	0x0A0A, 0x1A1A, 0x2A2A, 0x3A3A, 0x4A4A, 0x5A5A,
	0x6A6A, 0x7A7A, 0x8A8A, 0x9A9A, 0xAAAA, 0xBABA,
	0xCACA, 0xDADA, 0xEAEA, 0xFAFA,
}

func randomGREASE() uint16 {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(greaseValues))))
	return greaseValues[n.Int64()]
}

type Profile struct {
	Name           string
	CipherSuites   []uint16
	SupportedGroups []uint16
	SigAlgs        []uint16
	ALPNProtos     []string
	TLSVersions    []uint16
	PSKModes       []byte
	CompressCert   []byte
	UseGREASE      bool
	ExtensionOrder []uint16
}

const (
	extSNI               = 0x0000
	extStatusRequest     = 0x0005
	extSupportedGroups   = 0x000A
	extECPointFormats    = 0x000B
	extSigAlgs           = 0x000D
	extALPN              = 0x0010
	extSignedCertTS      = 0x0012
	extPadding           = 0x0015
	extExtendedMasterSec = 0x0017
	extCompressCert      = 0x001B
	extSessionTicket     = 0x0023
	extSupportedVersions = 0x002B
	extPSKKeyExchModes   = 0x002D
	extKeyShare          = 0x0033
	extRenegotiationInfo = 0xFF01
)

var Chrome124 = &Profile{
	Name: "Chrome/124",
	CipherSuites: []uint16{
		0x1301, 0x1302, 0x1303,
		0xC02B, 0xC02F, 0xC02C, 0xC030,
		0xCCA9, 0xCCA8,
		0xC013, 0xC014,
		0x009C, 0x009D,
		0x002F, 0x0035,
	},
	SupportedGroups: []uint16{0x001D, 0x0017, 0x0018, 0x0019},
	SigAlgs: []uint16{
		0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501,
		0x0806, 0x0601, 0x0201,
	},
	ALPNProtos:   []string{"h2", "http/1.1"},
	TLSVersions:  []uint16{0x0304, 0x0303},
	PSKModes:     []byte{0x01},
	CompressCert: []byte{0x00, 0x02, 0x00, 0x02},
	UseGREASE:    true,
	ExtensionOrder: []uint16{
		extSNI, extExtendedMasterSec, extRenegotiationInfo,
		extSupportedGroups, extECPointFormats, extSessionTicket,
		extALPN, extStatusRequest, extSignedCertTS,
		extKeyShare, extSupportedVersions, extCompressCert,
		extPSKKeyExchModes, extPadding,
	},
}

var Firefox125 = &Profile{
	Name: "Firefox/125",
	CipherSuites: []uint16{
		0x1301, 0x1303, 0x1302,
		0xC02B, 0xC02F, 0xCCA9, 0xCCA8,
		0xC02C, 0xC030,
		0xC00A, 0xC009, 0xC013, 0xC014,
		0x0033, 0x0039, 0x002F, 0x0035,
	},
	SupportedGroups: []uint16{0x001D, 0x0017, 0x0018, 0x0019},
	SigAlgs: []uint16{
		0x0403, 0x0503, 0x0603, 0x0804, 0x0805, 0x0806,
		0x0401, 0x0501, 0x0601, 0x0203, 0x0201, 0x0303, 0x0301,
	},
	ALPNProtos:  []string{"h2", "http/1.1"},
	TLSVersions: []uint16{0x0304, 0x0303},
	PSKModes:    []byte{0x01},
	UseGREASE:   false,
	ExtensionOrder: []uint16{
		extSNI, extExtendedMasterSec, extSupportedGroups,
		extECPointFormats, extSessionTicket, extStatusRequest,
		extALPN, extSigAlgs, extKeyShare,
		extSupportedVersions, extPSKKeyExchModes, extPadding,
	},
}

var Safari17 = &Profile{
	Name: "Safari/17",
	CipherSuites: []uint16{
		0xC02C, 0xC02B, 0xC030, 0xC02F,
		0x1302, 0x1303, 0x1301,
		0xCCA9, 0xCCA8,
		0xC014, 0xC013,
		0x009D, 0x009C,
		0x0035, 0x002F,
	},
	SupportedGroups: []uint16{0x001D, 0x0017, 0x0018},
	SigAlgs: []uint16{
		0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501,
		0x0806, 0x0601,
	},
	ALPNProtos:  []string{"h2", "http/1.1"},
	TLSVersions: []uint16{0x0304, 0x0303},
	PSKModes:    []byte{0x01},
	UseGREASE:   false,
	ExtensionOrder: []uint16{
		extSNI, extExtendedMasterSec, extSupportedGroups,
		extECPointFormats, extSessionTicket, extALPN,
		extStatusRequest, extSigAlgs, extKeyShare,
		extSupportedVersions, extPSKKeyExchModes, extPadding,
	},
}

var Edge124 = &Profile{
	Name: "Edge/124",
	CipherSuites: []uint16{
		0x1301, 0x1302, 0x1303,
		0xC02B, 0xC02F, 0xC02C, 0xC030,
		0xCCA9, 0xCCA8,
		0xC013, 0xC014,
		0x009C, 0x009D,
		0x002F, 0x0035,
	},
	SupportedGroups: []uint16{0x001D, 0x0017, 0x0018, 0x0019, 0x0100},
	SigAlgs: []uint16{
		0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501,
		0x0806, 0x0601, 0x0201,
	},
	ALPNProtos:   []string{"h2", "http/1.1"},
	TLSVersions:  []uint16{0x0304, 0x0303},
	PSKModes:     []byte{0x01},
	CompressCert: []byte{0x00, 0x02, 0x00, 0x02},
	UseGREASE:    true,
	ExtensionOrder: []uint16{
		extSNI, extExtendedMasterSec, extRenegotiationInfo,
		extSupportedGroups, extECPointFormats, extSessionTicket,
		extALPN, extStatusRequest, extSignedCertTS,
		extKeyShare, extSupportedVersions, extCompressCert,
		extPSKKeyExchModes, extPadding,
	},
}

var Chrome120 = &Profile{
	Name: "Chrome/120",
	CipherSuites: []uint16{
		0x1301, 0x1302, 0x1303,
		0xC02B, 0xC02F, 0xC02C, 0xC030,
		0xCCA9, 0xCCA8,
		0xC013, 0xC014,
		0x009C, 0x009D,
		0x002F, 0x0035,
	},
	SupportedGroups: []uint16{0x001D, 0x0017, 0x0018},
	SigAlgs: []uint16{
		0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501,
		0x0806, 0x0601, 0x0201,
	},
	ALPNProtos:  []string{"h2", "http/1.1"},
	TLSVersions: []uint16{0x0304, 0x0303},
	PSKModes:    []byte{0x01},
	UseGREASE:   true,
	ExtensionOrder: []uint16{
		extSNI, extExtendedMasterSec, extRenegotiationInfo,
		extSupportedGroups, extECPointFormats, extSessionTicket,
		extALPN, extStatusRequest, extSignedCertTS,
		extKeyShare, extSupportedVersions,
		extPSKKeyExchModes, extPadding,
	},
}

var allProfiles = []*Profile{Chrome124, Firefox125, Safari17, Edge124, Chrome120}

var profileMap = map[string]*Profile{
	"chrome":  Chrome124,
	"firefox": Firefox125,
	"safari":  Safari17,
	"edge":    Edge124,
}

var rng struct {
	sync.Mutex
	r *mrand.Rand
}

func init() {
	var seed [8]byte
	rand.Read(seed[:])
	rng.r = mrand.New(mrand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))
}

func fastRandN(n int) int {
	rng.Lock()
	v := rng.r.Intn(n)
	rng.Unlock()
	return v
}

func GetProfile(name string) *Profile {
	if name == "random" || name == "" {
		return allProfiles[fastRandN(len(allProfiles))]
	}
	if p, ok := profileMap[name]; ok {
		return p
	}
	return Chrome124
}
