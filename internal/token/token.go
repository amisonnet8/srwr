// Package token encodes and decodes selection tokens ("sel_…", docs/design/token.md).
//
// A token is a version byte, three LEB128 integers (seq, startLine, endLine), two
// 4-byte hashes (file path, range text) and a 3-byte HMAC, written in Crockford Base32.
// The HMAC covers the whole tape ID, so a token is only valid in the tape that issued it.
package token

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
)

const (
	prefix  = "sel_"
	version = 1
	macLen  = 3
	// alphabet is Crockford Base32: no I, L, O or U.
	alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// ErrInvalid is returned for every token that cannot be trusted: malformed, altered, or issued in another tape.
var ErrInvalid = errors.New("invalid selection token")

// Token is what a selection token carries.
type Token struct {
	Seq       uint64 // seq of the event that issued the token
	StartLine uint64
	EndLine   uint64 // StartLine-1 for an empty range
	FileHash  [4]byte
	TextHash  [4]byte
}

// Hash4 returns the first 4 bytes of the SHA-256 of s. It is used for both FileHash and TextHash.
func Hash4(s string) [4]byte {
	sum := sha256.Sum256([]byte(s))
	return [4]byte(sum[:4])
}

// Encode returns the token string. tapeID is the whole tape ID, such as "20261001-1706-1795".
func Encode(t Token, tapeID string, key []byte) string {
	body := t.body()
	b := append(body, mac(key, tapeID, body)...)
	return prefix + base32Encode(b)
}

// Decode checks the token against the tape ID and key, and returns what it carries.
func Decode(s, tapeID string, key []byte) (Token, error) {
	t, body, got, err := decodeBody(s)
	if err != nil {
		return Token{}, err
	}
	if !hmac.Equal(got, mac(key, tapeID, body)) {
		return Token{}, ErrInvalid
	}
	return t, nil
}

// decodeBody parses the token without checking the HMAC. It returns the body that the HMAC covers and the HMAC itself.
func decodeBody(s string) (t Token, body, got []byte, err error) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return Token{}, nil, nil, ErrInvalid
	}
	raw, ok := base32Decode(s[len(prefix):])
	if !ok || len(raw) < 1+3+8+macLen || raw[0] != version {
		return Token{}, nil, nil, ErrInvalid
	}
	rest := raw[1:]
	var nums [3]uint64
	for i := range nums {
		v, n := binary.Uvarint(rest)
		// Reject an overlong encoding as well: only the shortest form is ever written.
		if n <= 0 || n != len(binary.AppendUvarint(nil, v)) {
			return Token{}, nil, nil, ErrInvalid
		}
		nums[i] = v
		rest = rest[n:]
	}
	if len(rest) != 8+macLen {
		return Token{}, nil, nil, ErrInvalid
	}
	t = Token{Seq: nums[0], StartLine: nums[1], EndLine: nums[2], FileHash: [4]byte(rest[:4]), TextHash: [4]byte(rest[4:8])}
	return t, raw[:len(raw)-macLen], rest[8:], nil
}

func (t Token) body() []byte {
	b := []byte{version}
	b = binary.AppendUvarint(b, t.Seq)
	b = binary.AppendUvarint(b, t.StartLine)
	b = binary.AppendUvarint(b, t.EndLine)
	b = append(b, t.FileHash[:]...)
	return append(b, t.TextHash[:]...)
}

func mac(key []byte, tapeID string, body []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(tapeID))
	h.Write(body)
	return h.Sum(nil)[:macLen]
}

// base32Encode writes the bits most significant first, five at a time, and pads the last group with zeros.
func base32Encode(b []byte) string {
	var out strings.Builder
	var acc uint32
	var bits uint
	for _, x := range b {
		acc = acc<<8 | uint32(x)
		bits += 8
		for bits >= 5 {
			out.WriteByte(alphabet[acc>>(bits-5)&31])
			bits -= 5
		}
	}
	if bits > 0 {
		out.WriteByte(alphabet[acc<<(5-bits)&31])
	}
	return out.String()
}

// base32Decode reads Crockford Base32 case-insensitively, reading O as 0 and I and L as 1, and ignoring hyphens.
// The padding bits at the end must be zero, so that one value has one spelling.
func base32Decode(s string) ([]byte, bool) {
	var out bytes.Buffer
	var acc int
	var bits uint
	for _, r := range strings.ToUpper(s) {
		switch r {
		case '-':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		v := strings.IndexRune(alphabet, r)
		if v < 0 {
			return nil, false
		}
		acc = (acc<<5 | v) & 0x1fff // at most 7 bits are left over before the next five arrive
		bits += 5
		if bits >= 8 {
			out.WriteByte(byte(acc >> (bits - 8) & 0xff))
			bits -= 8
		}
	}
	// Five or more bits left over would be a whole extra character.
	if bits >= 5 || acc&(1<<bits-1) != 0 {
		return nil, false
	}
	return out.Bytes(), true
}
