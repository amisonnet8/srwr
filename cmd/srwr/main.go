// Command srwr is the single binary of srwr. The work is in internal/cli.
package main

import (
	"os"

	"github.com/amisonnet8/srwr/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
