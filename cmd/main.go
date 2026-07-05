package main

import (
	"fmt"
	"os"

	"github.com/merlincox/wheeler/internal/runner"
)

var (
	version  string
	defaults = runner.DefaultConfig()
)

func main() {
	if err := runner.Run(defaults, version); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
