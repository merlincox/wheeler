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
	cfg := defaults
	if err := runner.Run(cfg, version); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
