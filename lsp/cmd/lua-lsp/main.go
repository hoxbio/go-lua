package main

import (
	"os"

	"github.com/hoxbio/go-lua/lsp"
)

func main() {
	lsp.NewServer(os.Stdin, os.Stdout).Run()
}
