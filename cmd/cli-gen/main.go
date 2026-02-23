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
	if err := RunGenerator(flag.Arg(0), *out); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
