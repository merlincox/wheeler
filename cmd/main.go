package main

import (
	"fmt"
	"os"

	"github.com/merlincox/wheeler/internal/runner"
)

var version string

func main() {
	if err := runner.Run(version); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
