package main

import (
	"bufio"
	"os"
)

func main() {
	s := &Server{
		docs: make(map[string]*Document),
		in:   bufio.NewReader(os.Stdin),
		out:  bufio.NewWriter(os.Stdout),
	}
	s.Run()
}
