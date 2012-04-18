package main

import (
	"binidl"
	"flag"
	"fmt"
	"os"
)

func usage() {
	fmt.Println("usage:  bi [-B] <input file.go>")
}

var bigEndian *bool = flag.Bool("B", false, "Use big endian encoding (default: little)")

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
		os.Exit(-1)
	}

	bi := binidl.NewBinidl(flag.Arg(0), *bigEndian)
	bi.PrintGo()
}
