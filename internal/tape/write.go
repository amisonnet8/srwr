package tape

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// field is one key of an event, written in the order of docs/reference/tape.md.
type field struct {
	key string
	val any
}

// Marshal returns the tape line for e, including the final newline. Every field of the
// type is written, with null for a missing value, as the format requires.
func Marshal(e Event) ([]byte, error) {
	head := []field{{"v", Version}}
	var fs []field
	switch e.Type {
	case TypeHeader:
		fs = []field{{"type", e.Type}, {"session", e.Session}, {"startedAt", e.StartedAt}, {"author", e.Author}, {"vcs", e.VCS}, {"tool", e.Tool}}
	case TypeSnapshot:
		if e.Text == nil {
			return nil, fmt.Errorf("snapshot of %s has no text", e.File)
		}
		fs = []field{{"seq", e.Seq}, {"ts", e.TS}, {"type", e.Type}, {"file", e.File}, {"fileHash", e.FileHash}, {"text", e.Text}, {"sha", e.Sha}}
	case TypeSelect:
		fs = []field{
			{"seq", e.Seq}, {"ts", e.TS}, {"type", e.Type}, {"file", e.File},
			{"startLine", e.StartLine}, {"endLine", e.EndLine}, {"why", e.Why}, {"selection", e.Selection},
		}
		fs = appendSource(fs, e)
	case TypeReplace:
		fs = []field{
			{"seq", e.Seq}, {"ts", e.TS}, {"type", e.Type}, {"file", e.File}, {"from", e.From},
			{"startLine", e.StartLine}, {"endLine", e.EndLine}, {"oldText", e.OldText}, {"newText", e.NewText},
			{"newStartLine", e.NewStartLine}, {"newEndLine", e.NewEndLine}, {"selection", e.Selection}, {"why", e.Why},
			{"fileShaBefore", e.FileShaBefore}, {"fileShaAfter", e.FileShaAfter},
		}
		fs = appendSource(fs, e)
	case TypeExternal:
		fs = []field{
			{"seq", e.Seq}, {"ts", e.TS}, {"type", e.Type}, {"file", e.File}, {"author", e.Author},
			{"detectedBy", e.DetectedBy}, {"expectedSha", e.ExpectedSha}, {"actualSha", e.ActualSha},
		}
		if e.Hunks != nil {
			fs = append(fs, field{"hunks", e.Hunks})
		} else {
			fs = append(fs, field{"text", e.Text})
		}
		if e.Created {
			fs = append(fs, field{"created", true})
		}
		if e.Deleted {
			fs = append(fs, field{"deleted", true})
		}
	default:
		return nil, fmt.Errorf("unknown event type %q", e.Type)
	}
	return object(append(head, fs...))
}

func appendSource(fs []field, e Event) []field {
	if e.Source != "" {
		fs = append(fs, field{"source", e.Source})
	}
	if e.HookTool != "" {
		fs = append(fs, field{"tool", e.HookTool})
	}
	return fs
}

func object(fs []field) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range fs {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		// Not json.Marshal: it escapes <, > and & in the text of the files, which a reader of the tape does not want.
		var v bytes.Buffer
		enc := json.NewEncoder(&v)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(f.val); err != nil {
			return nil, err
		}
		buf.Write(bytes.TrimSuffix(v.Bytes(), []byte{'\n'}))
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

// Append writes e as one line at the end of the tape file, in a single write call.
// It creates the file if needed. Callers hold the session lock (internal/session).
func Append(path string, e Event) error {
	line, err := Marshal(e)
	if err != nil {
		return err
	}
	return AppendLine(path, line)
}

// AppendLine writes an already marshaled line (Marshal) with a single write call.
func AppendLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600) //nolint:gosec // the tape path is built by the caller from the workspace
	if err != nil {
		return err
	}
	n, err := f.Write(line)
	if err == nil && n < len(line) {
		err = io.ErrShortWrite
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// FormatTS formats a time as the tape's ts: RFC 3339 in UTC (a trailing Z) with milliseconds. Older tapes were written in
// the zone of the machine (+09:00 and so on); both are read as the same instant.
func FormatTS(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// Sha returns the "sha256:…" of text, as in sha, fileShaBefore and fileShaAfter.
func Sha(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// FileHash returns the fileHash of a snapshot: the first 4 bytes of the SHA-256 of the
// workspace-relative path, as 8 hexadecimal digits.
func FileHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:4])
}
