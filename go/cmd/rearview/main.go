package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(_ []string, out io.Writer) int {
	fmt.Fprintf(out, "rearview %s\n", version)
	return 0
}
