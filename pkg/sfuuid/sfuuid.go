// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package sfuuid converts LFX Salesforce IDs to/from UUID v8 (RFC 9562).
//
// Bit layout within the 122 custom bits of UUID v8:
//
//	[48: custom_a        | 4: ver=8 | 12: custom_b | 2: var=10₂ | 62: custom_c        ]
//	[NS32 (32) | sfid[89:74] (16)   | sfid[73:62]  |            | sfid[61:0]          ]
//	 \______ custom_a (48) _______/   \_ custom_b _/             \___ custom_c (62) __/
//
// NS32 is the ASCII bytes of "LFX_" as a big-endian uint32:
//
//	'L'=0x4C  'F'=0x46  'X'=0x58  '_'=0x5F  →  0x4C46585F
//
// Salesforce base-62 alphabet: A-Z (0-25), a-z (26-51), 0-9 (52-61).
//
// Because the mapping is lossless and deterministic, UUIDs produced by ToUUID
// can be reversed back to the original 15-char Salesforce ID via ToSFID, making
// them suitable as opaque API identifiers that the service can decode without a
// database lookup.
package sfuuid

import (
	"fmt"
	"math/big"
	"strings"
)

// b62Chars is the Salesforce base-62 alphabet. Order must match across all implementations.
const b62Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// suffixChars maps a 5-bit value (0-31) to its suffix character for 18-char IDs.
const suffixChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"

// ns32 is the 4-byte namespace fingerprint: ASCII "LFX_" as a big-endian uint32.
//
//	'L'=0x4C  'F'=0x46  'X'=0x58  '_'=0x5F  →  0x4C46585F
const ns32 uint32 = 0x4C46585F

// b62Index maps ASCII byte → base-62 index (-1 = invalid character).
var b62Index [256]int8

func init() {
	for i := range b62Index {
		b62Index[i] = -1
	}
	for i, ch := range b62Chars {
		b62Index[byte(ch)] = int8(i)
	}
}

var bigBase = big.NewInt(62)

// ToUUID encodes a Salesforce ID (15- or 18-char) as a reversible LFX_ UUID v8
// string. The result is stable and deterministic: the same SFID always produces
// the same UUID. The UUID can be decoded back to the original 15-char SFID via
// ToSFID.
func ToUUID(sfid string) (string, error) {
	u, err := sfidToUUID(sfid)
	if err != nil {
		return "", err
	}
	return u.string(), nil
}

// ToSFID decodes an LFX_ UUID v8 string back to its canonical 15-char
// Salesforce ID. Returns an error if the string is not a valid LFX_ UUID v8.
func ToSFID(uuidStr string) (string, error) {
	u, err := parseUUID(uuidStr)
	if err != nil {
		return "", err
	}
	return uuidToSFID(u)
}

// IsSFID reports whether s is a syntactically valid 15- or 18-character
// Salesforce ID (all characters in the base-62 alphabet, with the 18-char form
// including a valid 3-character case-encoding suffix).
func IsSFID(s string) bool {
	_, err := normalize(s)
	return err == nil
}

// Salesforce15To18 appends the standard 3-character case-encoding suffix to a
// 15-char Salesforce ID, producing the portable 18-char case-insensitive form.
//
// The suffix encodes which positions in the base ID hold uppercase letters.
// Characters are grouped into three sets of five; each group yields one suffix
// character via a 5-bit mask (bit j set ↔ position j is uppercase A-Z), mapped
// through suffixChars ("ABCDEFGHIJKLMNOPQRSTUVWXYZ012345").
func Salesforce15To18(id15 string) (string, error) {
	if len(id15) != 15 {
		return "", fmt.Errorf("uid: expected 15-char Salesforce ID, got %d chars", len(id15))
	}
	var sb strings.Builder
	sb.Grow(18)
	sb.WriteString(id15)
	for group := 0; group < 3; group++ {
		var bits int
		for j := 0; j < 5; j++ {
			ch := id15[group*5+j]
			if ch >= 'A' && ch <= 'Z' {
				bits |= 1 << j
			}
		}
		sb.WriteByte(suffixChars[bits])
	}
	return sb.String(), nil
}

// ── internal UUID type ────────────────────────────────────────────────────────

// uuid16 is a raw 16-byte UUID value.
type uuid16 [16]byte

// string returns the standard 8-4-4-4-12 hyphenated hex form.
func (u uuid16) string() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// parseUUID parses a standard 8-4-4-4-12 hyphenated hex UUID string.
func parseUUID(s string) (uuid16, error) {
	// Accept both with and without hyphens (36 or 32 chars).
	clean := strings.ReplaceAll(s, "-", "")
	if len(clean) != 32 {
		return uuid16{}, fmt.Errorf("invalid UUID length %d", len(s))
	}
	var u uuid16
	for i := 0; i < 16; i++ {
		hi := hexVal(clean[i*2])
		lo := hexVal(clean[i*2+1])
		if hi < 0 || lo < 0 {
			return uuid16{}, fmt.Errorf("invalid hex character in UUID %q", s)
		}
		u[i] = byte(hi<<4 | lo)
	}
	return u, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

// ── Salesforce ID helpers ─────────────────────────────────────────────────────

// normalize returns the canonical 15-char form, stripping the 3-char suffix if
// an 18-char ID is provided, and validating all base-62 characters.
func normalize(sfid string) (string, error) {
	if len(sfid) == 18 {
		sfid = sfid[:15]
	}
	if len(sfid) != 15 {
		return "", fmt.Errorf("uid: expected 15- or 18-char Salesforce ID, got %d chars", len(sfid))
	}
	for i := 0; i < len(sfid); i++ {
		if b62Index[sfid[i]] < 0 {
			return "", fmt.Errorf("uid: invalid character %q at position %d", sfid[i], i)
		}
	}
	return sfid, nil
}

// ── base-62 codec ─────────────────────────────────────────────────────────────

func b62Decode(s string) *big.Int {
	n := new(big.Int)
	for i := 0; i < len(s); i++ {
		n.Mul(n, bigBase)
		n.Add(n, big.NewInt(int64(b62Index[s[i]])))
	}
	return n
}

func b62Encode(n *big.Int, width int) string {
	if n.Sign() == 0 {
		return strings.Repeat(string(b62Chars[0]), width)
	}
	base := big.NewInt(62)
	mod := new(big.Int)
	tmp := new(big.Int).Set(n)
	digits := make([]byte, 0, width)
	for tmp.Sign() > 0 {
		tmp.DivMod(tmp, base, mod)
		digits = append(digits, b62Chars[mod.Int64()])
	}
	// Reverse digits (DivMod produces least-significant first).
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	s := string(digits)
	if pad := width - len(s); pad > 0 {
		s = strings.Repeat(string(b62Chars[0]), pad) + s
	}
	return s
}

// ── core encode / decode ──────────────────────────────────────────────────────

// sfidToUUID encodes a Salesforce ID (15- or 18-char) as an LFX_ UUID v8.
func sfidToUUID(sfid string) (uuid16, error) {
	sfid, err := normalize(sfid)
	if err != nil {
		return uuid16{}, err
	}

	v := b62Decode(sfid) // ≤90 bits

	// Partition the 90-bit integer into three fields.
	mask62 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 62), big.NewInt(1))
	sfidLow := new(big.Int).And(v, mask62)                                  // bits 61:0  → 62 bits
	sfidMid := new(big.Int).And(new(big.Int).Rsh(v, 62), big.NewInt(0xFFF)) // bits 73:62 → 12 bits
	sfidHigh := new(big.Int).Rsh(v, 74)                                     // bits 89:74 → ≤16 bits

	customA := uint64(ns32)<<16 | sfidHigh.Uint64() // 48 bits: NS32 ‖ sfid high
	customB := uint16(sfidMid.Uint64())             // 12 bits
	customC := sfidLow.Uint64()                     // 62 bits

	// Pack into 16 bytes — UUID v8 wire format (big-endian):
	//   bytes 0-5  : custom_a (48 bits)
	//   byte  6    : ver=8 (high nibble) | custom_b[11:8] (low nibble)
	//   byte  7    : custom_b[7:0]
	//   byte  8    : var=10₂ (high 2 bits) | custom_c[61:56] (low 6 bits)
	//   bytes 9-15 : custom_c[55:0]
	var out uuid16
	out[0] = byte(customA >> 40)
	out[1] = byte(customA >> 32)
	out[2] = byte(customA >> 24)
	out[3] = byte(customA >> 16)
	out[4] = byte(customA >> 8)
	out[5] = byte(customA)
	out[6] = 0x80 | byte(customB>>8) // 0x8_ (version 8)
	out[7] = byte(customB)
	out[8] = 0x80 | byte(customC>>56) // 0b10xx_xxxx (RFC 9562 variant)
	out[9] = byte(customC >> 48)
	out[10] = byte(customC >> 40)
	out[11] = byte(customC >> 32)
	out[12] = byte(customC >> 24)
	out[13] = byte(customC >> 16)
	out[14] = byte(customC >> 8)
	out[15] = byte(customC)
	return out, nil
}

// uuidToSFID decodes an LFX_ UUID v8 back to its canonical 15-char Salesforce ID.
func uuidToSFID(u uuid16) (string, error) {
	customA := uint64(u[0])<<40 | uint64(u[1])<<32 | uint64(u[2])<<24 |
		uint64(u[3])<<16 | uint64(u[4])<<8 | uint64(u[5])

	if ver := u[6] >> 4; ver != 8 {
		return "", fmt.Errorf("uid: not a UUID v8 (version=%d)", ver)
	}
	customB := uint16(u[6]&0x0F)<<8 | uint16(u[7])

	if varBits := u[8] >> 6; varBits != 0b10 {
		return "", fmt.Errorf("uid: not RFC 9562 variant (variant=%02b)", varBits)
	}
	customC := uint64(u[8]&0x3F)<<56 | uint64(u[9])<<48 | uint64(u[10])<<40 |
		uint64(u[11])<<32 | uint64(u[12])<<24 | uint64(u[13])<<16 |
		uint64(u[14])<<8 | uint64(u[15])

	if ns := uint32(customA >> 16); ns != ns32 {
		return "", fmt.Errorf("uid: namespace mismatch — not an LFX_ Salesforce UUID (ns=0x%08X)", ns)
	}

	// Reconstruct the 90-bit Salesforce integer.
	sfidHigh := customA & 0xFFFF
	v := new(big.Int).SetUint64(sfidHigh)
	v.Lsh(v, 74)
	v.Or(v, new(big.Int).Lsh(new(big.Int).SetUint64(uint64(customB)), 62))
	v.Or(v, new(big.Int).SetUint64(customC))

	return b62Encode(v, 15), nil
}
