package main

import (
	"fmt"
	"os"

	"github.com/merlincox/wheeler/internal/runner"
)

func main() {
	if err := runner.Run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
