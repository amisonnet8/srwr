package tape

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const sample = "{\"v\":1,\"seq\":1,\"type\":\"header\"}\n{\"v\":1,\"seq\":2,\"type\":\"select\"}\n"

func TestCompressRoundTrip(t *testing.T) {
	dir := t.TempDir()
	plain := writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(sample))
	when := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(plain, when, when); err != nil {
		t.Fatal(err)
	}
	if err := Compress(dir, "20261001-0000-aaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plain); !os.IsNotExist(err) {
		t.Errorf("the plain tape is still there: %v", err)
	}
	got, err := ReadAll(dir, "20261001-0000-aaaa")
	if err != nil || string(got) != sample {
		t.Fatalf("ReadAll = %q, %v", got, err)
	}
	gz := plain + GzSuffix
	info, err := os.Stat(gz)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(when) {
		t.Errorf("the compressed tape changed at %v, want the plain tape's %v", info.ModTime(), when)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("files left: %v", entries)
	}
	// Anyone with gzip can read it.
	f, _ := os.Open(gz) //nolint:gosec // a temporary directory
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(zr)
	_ = f.Close()
	if buf.String() != sample {
		t.Errorf("gzip reads %q", buf.String())
	}
}

func TestCompressIsDeterministic(t *testing.T) {
	var outs [][]byte
	for range 2 {
		dir := t.TempDir()
		p := writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(sample))
		later := time.Now().Add(time.Hour)
		_ = os.Chtimes(p, later, later) // the time of the plain tape is not in the compressed bytes
		if err := Compress(dir, "20261001-0000-aaaa"); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p + GzSuffix) //nolint:gosec // a temporary directory
		if err != nil {
			t.Fatal(err)
		}
		outs = append(outs, b)
	}
	if !bytes.Equal(outs[0], outs[1]) {
		t.Error("the same tape compressed to different bytes")
	}
}

func TestCompressOfNothingAndOfAClosedTape(t *testing.T) {
	dir := t.TempDir()
	if err := Compress(dir, "20261001-0000-aaaa"); err != nil {
		t.Errorf("no tape: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("made %v out of nothing", entries)
	}
	writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(sample))
	if err := Compress(dir, "20261001-0000-aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := Compress(dir, "20261001-0000-aaaa"); err != nil { // again: nothing left to do
		t.Errorf("a closed tape: %v", err)
	}
	if got, err := ReadAll(dir, "20261001-0000-aaaa"); err != nil || string(got) != sample {
		t.Errorf("ReadAll = %q, %v", got, err)
	}
}

// A crash after the compressed file was put down and before the plain one was removed leaves both. The plain one is the tape,
// and the next Compress finishes the job.
func TestPlainTapeWinsOverTheCompressedOne(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(sample+"{\"v\":1,\"seq\":3,\"type\":\"select\"}\n"))
	var zbuf bytes.Buffer
	zw := gzip.NewWriter(&zbuf)
	_, _ = zw.Write([]byte(sample))
	_ = zw.Close()
	writeFile(t, dir, FileName("20261001-0000-aaaa")+GzSuffix, zbuf.Bytes())

	got, err := ReadAll(dir, "20261001-0000-aaaa")
	if err != nil || !strings.Contains(string(got), `"seq":3`) {
		t.Fatalf("ReadAll took the compressed tape: %q, %v", got, err)
	}
	if err := Compress(dir, "20261001-0000-aaaa"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadAll(dir, "20261001-0000-aaaa"); !strings.Contains(string(got), `"seq":3`) {
		t.Errorf("the compressed tape was not made again from the plain one: %q", got)
	}
}

func TestIDsAndRemove(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(sample))
	writeFile(t, dir, FileName("20261001-0000-bbbb")+GzSuffix, []byte("x"))
	writeFile(t, dir, FileName("20261001-0000-cccc"), []byte(sample)) // both of these
	writeFile(t, dir, FileName("20261001-0000-cccc")+GzSuffix, []byte("x"))
	writeFile(t, dir, ".compress-123.tmp", []byte("x"))
	writeFile(t, dir, "notes.txt", []byte("x"))
	writeFile(t, dir, "..odd"+FileSuffix, []byte("x"))
	if err := os.Mkdir(filepath.Join(dir, "d"+FileSuffix), 0o750); err != nil {
		t.Fatal(err)
	}
	want := []string{"20261001-0000-aaaa", "20261001-0000-bbbb", "20261001-0000-cccc"}
	if got := IDs(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("IDs = %v, want %v", got, want)
	}
	if err := Remove(dir, "20261001-0000-cccc"); err != nil {
		t.Fatal(err)
	}
	if got := IDs(dir); !reflect.DeepEqual(got, want[:2]) {
		t.Errorf("after Remove: %v", got)
	}
	if err := Remove(dir, "20261001-0000-zzzz"); err != nil {
		t.Errorf("removing a tape that is not there: %v", err)
	}
}

func TestIDOf(t *testing.T) {
	for in, want := range map[string]string{
		"20261001-0000-aaaa":                                        "20261001-0000-aaaa",
		"20261001-0000-aaaa.tape.jsonl":                             "20261001-0000-aaaa",
		"20261001-0000-aaaa.tape.jsonl.gz":                          "20261001-0000-aaaa",
		filepath.Join("x", "y", "20261001-0000-aaaa.tape.jsonl.gz"): "20261001-0000-aaaa",
	} {
		if got := IDOf(in); got != want {
			t.Errorf("IDOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadAllMissingAndTooBig(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadAll(dir, "20261001-0000-aaaa"); !os.IsNotExist(unwrapped(err)) {
		t.Errorf("a tape that is not there: %v", err)
	}
	// A small file that expands past the limit is refused, not read into memory.
	if testing.Short() {
		t.Skip("expands 1 GiB")
	}
	var zbuf bytes.Buffer
	zw := gzip.NewWriter(&zbuf)
	chunk := make([]byte, 1<<20)
	for range MaxTapeSize/len(chunk) + 1 {
		_, _ = zw.Write(chunk)
	}
	_ = zw.Close()
	writeFile(t, dir, FileName("20261001-0000-bbbb")+GzSuffix, zbuf.Bytes())
	if _, err := ReadAll(dir, "20261001-0000-bbbb"); err == nil {
		t.Error("a tape that expands past the limit was read")
	}
}

func unwrapped(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok || u.Unwrap() == nil {
			return err
		}
		err = u.Unwrap()
	}
}

func TestCompressFailureLeavesThePlainTape(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only directories do not stop this user")
	}
	dir := t.TempDir()
	writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(sample))
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // a temporary directory
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }() //nolint:gosec // a temporary directory
	if err := Compress(dir, "20261001-0000-aaaa"); err == nil {
		t.Error("Compress succeeded in a directory it cannot write")
	}
	if got, err := ReadAll(dir, "20261001-0000-aaaa"); err != nil || string(got) != sample {
		t.Errorf("the plain tape is gone: %q, %v", got, err)
	}
}

func TestCompressMakesTheTapeSmaller(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for range 200 {
		b.WriteString(sample)
	}
	p := writeFile(t, dir, FileName("20261001-0000-aaaa"), []byte(b.String()))
	if err := Compress(dir, "20261001-0000-aaaa"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p + GzSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size()*4 > int64(b.Len()) {
		t.Errorf("%d bytes became %d: not compressed", b.Len(), info.Size())
	}
}
