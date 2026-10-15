package core

import (
	"fmt"
	"os"
	"testing"

	"github.com/amisonnet8/srwr/internal/gittest"
)

func TestMain(m *testing.M) {
	cleanup, err := gittest.Isolate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
