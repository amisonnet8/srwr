package token

import (
	"errors"
	"strings"
	"testing"
)

const tapeID = "20261001-1706-1795"

var key = []byte("0123456789abcdef0123456789abcdef")

func sample() Token {
	return Token{Seq: 7, StartLine: 12, EndLine: 14, FileHash: Hash4("cmd/app/main.go"), TextHash: Hash4("a\nb\nc")}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		tok  Token
	}{
		{"typical", sample()},
		{"zero seq and empty range", Token{Seq: 0, StartLine: 1, EndLine: 0}},
		{"one byte boundary", Token{Seq: 127, StartLine: 127, EndLine: 127}},
		{"two byte boundary", Token{Seq: 128, StartLine: 16383, EndLine: 16384}},
		{"large", Token{Seq: 1 << 40, StartLine: 1 << 33, EndLine: 1<<64 - 1}},
		{"hashes", Token{Seq: 1, StartLine: 1, EndLine: 1, FileHash: [4]byte{0xff, 0, 0xff, 1}, TextHash: [4]byte{0, 0xff, 2, 0xff}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Encode(tt.tok, tapeID, key)
			if !strings.HasPrefix(s, "sel_") {
				t.Fatalf("Encode = %q, want the sel_ prefix", s)
			}
			got, err := Decode(s, tapeID, key)
			if err != nil {
				t.Fatalf("Decode(%q): %v", s, err)
			}
			if got != tt.tok {
				t.Errorf("Decode(%q) = %+v, want %+v", s, got, tt.tok)
			}
		})
	}
}

func TestEncodeLength(t *testing.T) {
	// 15 bytes for a typical token: 24 characters after the prefix (docs/design/token.md says 23 to 32).
	if n := len(Encode(sample(), tapeID, key)) - len(prefix); n < 23 || n > 32 {
		t.Errorf("token has %d characters after the prefix, want 23 to 32", n)
	}
}

// normalize reads a token string the way Decode does, to tell a real change from an equivalent spelling.
func normalize(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(s, "-", ""))
	return strings.NewReplacer("O", "0", "I", "1", "L", "1").Replace(s)
}

func TestTamper(t *testing.T) {
	good := Encode(sample(), tapeID, key)

	// Changing any one character must be caught.
	for i := len(prefix); i < len(good); i++ {
		for _, c := range alphabet {
			if string(c) == good[i:i+1] {
				continue
			}
			bad := good[:i] + string(c) + good[i+1:]
			if normalize(bad) == normalize(good) {
				continue
			}
			if _, err := Decode(bad, tapeID, key); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Decode(%q) with character %d changed to %c: err = %v, want ErrInvalid", bad, i, c, err)
			}
		}
	}

	tests := []struct {
		name string
		s    string
		id   string
		key  []byte
	}{
		{"other tape", good, "20261001-1706-1796", key},
		{"other key", good, tapeID, []byte("another key another key another!")},
		{"short tape ID only", good, "1795", key},
		{"empty", "", tapeID, key},
		{"prefix only", "sel_", tapeID, key},
		{"no prefix", good[len(prefix):], tapeID, key},
		{"other prefix", "tok_" + good[len(prefix):], tapeID, key},
		{"truncated", good[:len(good)-1], tapeID, key},
		{"extra character", good + "0", tapeID, key},
		{"extra characters", good + "00", tapeID, key},
		{"unknown character", good[:6] + "U" + good[7:], tapeID, key},
		{"space", good[:6] + " " + good[7:], tapeID, key},
		{"not a token", "hello", tapeID, key},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decode(tt.s, tt.id, tt.key); !errors.Is(err, ErrInvalid) {
				t.Errorf("Decode(%q) err = %v, want ErrInvalid", tt.s, err)
			}
		})
	}
}

func TestDecodeBodyRejects(t *testing.T) {
	// Tokens with a valid HMAC can still have a bad body; those must be rejected too.
	sign := func(body []byte) string { return prefix + base32Encode(append(body, mac(key, tapeID, body)...)) }
	hashes := make([]byte, 8)
	tests := []struct {
		name string
		body []byte
	}{
		{"version 2", append([]byte{2, 1, 1, 1}, hashes...)},
		{"version 0", append([]byte{0, 1, 1, 1}, hashes...)},
		{"overlong integer", append([]byte{1, 0x81, 0x00, 1, 1}, hashes...)},
		{"extra byte", append([]byte{1, 1, 1, 1}, append(hashes, 0)...)},
		{"missing hash byte", append([]byte{1, 1, 1, 1}, hashes[:7]...)},
		{"unterminated integer", append([]byte{1, 0x80, 0x80, 0x80}, hashes...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decode(sign(tt.body), tapeID, key); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
	// The same helper does accept a well-formed body, so the cases above fail for the reason they name.
	if _, err := Decode(sign(append([]byte{1, 1, 1, 1}, hashes...)), tapeID, key); err != nil {
		t.Errorf("well-formed body: %v", err)
	}
}

func TestLenientSpelling(t *testing.T) {
	good := Encode(sample(), tapeID, key)
	body := good[len(prefix):]

	// A copy with every 0 written as O and every 1 as I or L must still be the same token.
	spell := func(m map[rune]string) string {
		var b strings.Builder
		for _, r := range body {
			if s, ok := m[r]; ok {
				b.WriteString(s)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	hyphenated := body[:4] + "-" + body[4:10] + "-" + body[10:]

	tests := []struct {
		name string
		s    string
	}{
		{"lower case", strings.ToLower(good)},
		{"upper prefix", "SEL_" + body},
		{"O for 0", "sel_" + spell(map[rune]string{'0': "O"})},
		{"o for 0", "sel_" + strings.ToLower(spell(map[rune]string{'0': "O"}))},
		{"I for 1", "sel_" + spell(map[rune]string{'1': "I"})},
		{"L for 1", "sel_" + spell(map[rune]string{'1': "L"})},
		{"hyphens", "sel_" + hyphenated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.s, tapeID, key)
			if err != nil || got != sample() {
				t.Errorf("Decode(%q) = %+v, %v; want %+v", tt.s, got, err, sample())
			}
		})
	}
}

func TestHMACCoversWholeTapeID(t *testing.T) {
	// The same short ID at the end of two tape IDs must not make a token valid in both.
	a := Encode(sample(), "20261001-1706-1795", key)
	if _, err := Decode(a, "20261002-0900-1795", key); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token from another tape with the same short ID decoded: err = %v", err)
	}
}
