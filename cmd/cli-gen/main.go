package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	out := flag.String("out", "", "output directory")
	flag.Parse()
	if flag.NArg() != 1 || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: cli-gen <schema-dir> --out <output-dir>")
		os.Exit(2)
	}

	fmt.Fprintf(os.Stderr, "TODO: generate from %s to %s\n", flag.Arg(0), *out)
}
