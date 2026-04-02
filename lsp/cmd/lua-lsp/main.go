package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hoxbio/go-lua/lsp"
)

const serverName = "lua-lsp"
const serverVersion = "0.2.0"

func main() {
	flag.Usage = func() {
		fmt.Printf(`%s - Lua LSP Server

Usage: %s [options]

LSP servers communicate over stdin/stdout using JSON-RPC.

Options:
  --version  Print version information
  --help     Print this help message

The server implements the Language Server Protocol (LSP) with Lua-specific
extensions for runtime state tracking (kernel state).

Features:
  - Autocompletion
  - Hover documentation
  - Signature help
  - Go-to-definition
  - Find references
  - Rename
  - Formatting
`, serverName, serverName)
	}

	// Parse command line flags
	for i, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" || arg == "-v" {
			fmt.Printf("%s version %s\n", serverName, serverVersion)
			return
		}
		if arg == "--help" || arg == "-help" || arg == "-h" || arg == "-?" {
			flag.Usage()
			return
		}
		if arg == "-*" || strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
			flag.Usage()
			os.Exit(1)
		}
		// Non-flag arguments are passed to flag.Parse for validation
		if i == len(os.Args)-2 {
			// Last argument before potential args we don't understand
			break
		}
	}

	lsp.NewServer(os.Stdin, os.Stdout).Run()
}
