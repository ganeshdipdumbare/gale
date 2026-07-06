package main

import (
	"fmt"
	"os"

	"github.com/ganeshdipdumbare/gale/cmd/gale/cmd"
)

var version = "dev"

func main() {
	root := cmd.NewRoot(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
