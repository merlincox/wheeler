package main

import (
	"fmt"
	"os"

	"github.com/merlincox/wheeler/internal/runner"
)

var (
	text, outputFilepath, fontFilepath, version string
	verbose, debug, silent                      bool
)

func main() {
	cfg := runner.Config{
		Text:           text,
		OutputFilepath: outputFilepath,
		FontFilepath:   fontFilepath,
		Version:        version,
		Verbose:        verbose,
		Debug:          debug,
		Silent:         silent,
	}

	if err := runner.Run(cfg); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
