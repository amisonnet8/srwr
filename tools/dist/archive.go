package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// file is one thing to put into an archive.
type file struct {
	name string
	data []byte
	mode int64
}

// epoch is the time of every entry, so that the same files make the same archive.
var epoch = time.Unix(0, 0).UTC()

// writeArchive writes files as a .tar.gz, or a .zip when the name ends with .zip.
func writeArchive(path string, files []file) error {
	var buf bytes.Buffer
	var err error
	if strings.HasSuffix(path, ".zip") {
		err = writeZip(&buf, files)
	} else {
		err = writeTarGz(&buf, files)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // a file for everyone to download
}

func writeTarGz(w io.Writer, files []file) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		h := &tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.data)), ModTime: epoch, Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(f.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeZip(w io.Writer, files []file) error {
	zw := zip.NewWriter(w)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: epoch}
		h.SetMode(os.FileMode(f.mode)) //nolint:gosec // 0644 or 0755
		fw, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := fw.Write(f.data); err != nil {
			return err
		}
	}
	return zw.Close()
}

// readArchive returns the files of a .tar.gz or .zip by name.
func readArchive(path string) (map[string][]byte, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a path made by this tool
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	if strings.HasSuffix(path, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, err
			}
			out[f.Name] = data
		}
		return out, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		out[h.Name] = data
	}
}

// checkArchive checks an archive of t: it holds the executable (for the right OS and CPU), LICENSE and README.md.
func checkArchive(path string, t target) error {
	files, err := readArchive(path)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	exe, ok := files[t.exe()]
	if !ok {
		return fmt.Errorf("%s: no %s inside", filepath.Base(path), t.exe())
	}
	if err := checkBinary(exe, t); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	for _, name := range []string{"LICENSE", "README.md"} {
		if len(files[name]) == 0 {
			return fmt.Errorf("%s: no %s inside", filepath.Base(path), name)
		}
	}
	return nil
}

// writeChecksums writes checksums.txt for the named files of dir, in the form `sha256sum -c` reads.
func writeChecksums(dir string, names []string) error {
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n)) //nolint:gosec // a file this tool made
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), n)
	}
	return os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(b.String()), 0o644) //nolint:gosec // for everyone to download
}

// verifyChecksums checks every line of checksums.txt against the file, and that no file of dir is left out.
func verifyChecksums(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "checksums.txt")) //nolint:gosec // a file this tool made
	if err != nil {
		return err
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		sum, name, ok := strings.Cut(line, "  ")
		if !ok {
			return fmt.Errorf("checksums.txt: a line is %q", line)
		}
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // see above
		if err != nil {
			return fmt.Errorf("checksums.txt lists %s: %w", name, err)
		}
		got := sha256.Sum256(b)
		if hex.EncodeToString(got[:]) != sum {
			return fmt.Errorf("checksums.txt: %s does not match its SHA-256", name)
		}
		listed[name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() != "checksums.txt" && !listed[e.Name()] {
			return fmt.Errorf("checksums.txt does not list %s", e.Name())
		}
	}
	return nil
}
