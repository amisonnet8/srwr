package viewserver

import (
	"bufio"
	"io"
	"testing"
	"time"
)

// A connection kept open, so that a test can send a request, read the answer, and send more.

type pipeIn struct {
	t *testing.T
	w *io.PipeWriter
}

func (p pipeIn) send(line string) {
	p.t.Helper()
	if _, err := io.WriteString(p.w, line+"\n"); err != nil {
		p.t.Fatalf("send: %v", err)
	}
}

func (p pipeIn) close() { _ = p.w.Close() }

type pipeOut struct {
	t     *testing.T
	lines chan string
	done  chan error
}

// next returns the next line the server wrote. It gives up after a while, so that a missing line fails the test and does not hang it.
func (o pipeOut) next() string {
	o.t.Helper()
	select {
	case l, ok := <-o.lines:
		if !ok {
			o.t.Fatal("the server ended")
		}
		return l
	case <-time.After(10 * time.Second):
		o.t.Fatal("no line from the server")
		return ""
	}
}

// within returns the next line, or false if none comes within d.
func (o pipeOut) within(d time.Duration) (string, bool) {
	select {
	case l, ok := <-o.lines:
		return l, ok
	case <-time.After(d):
		return "", false
	}
}

// wait waits for Serve to return, and checks that it returned without an error.
func (o pipeOut) wait() {
	o.t.Helper()
	select {
	case err := <-o.done:
		if err != nil {
			o.t.Fatalf("Serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		o.t.Fatal("Serve did not return")
	}
}

func pipe(t *testing.T, srv *Server) (pipeIn, pipeOut) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	o := pipeOut{t: t, lines: make(chan string, 1000), done: make(chan error, 1)}
	go func() {
		err := srv.Serve(inR, outW)
		_ = outW.Close()
		o.done <- err
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<26)
		for sc.Scan() {
			o.lines <- sc.Text()
		}
		close(o.lines)
	}()
	t.Cleanup(func() { _ = inW.Close() })
	return pipeIn{t: t, w: inW}, o
}
