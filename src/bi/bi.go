package main

import (
	"binidl"
	"flag"
	"fmt"
	"os"
)

func usage() {
	fmt.Println("usage:  bi [-hgc] <input file.go>")
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
		os.Exit(-1)
	}

	bff := binidl.NewBinidl(flag.Arg(0))
	bff.PrintGo()
}
