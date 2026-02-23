package main

import (
	"fmt"
	"os"
)

func main() {
	schemaDir, outDir, ok := parseArgs(os.Args[1:])
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: cli-gen <schema-dir> --out <output-dir>")
		os.Exit(2)
	}
	if err := RunGenerator(schemaDir, outDir); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func parseArgs(args []string) (schemaDir, outDir string, ok bool) {
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--out" {
			if i+1 >= len(args) {
				return "", "", false
			}
			outDir = args[i+1]
			i++
			continue
		}
		positional = append(positional, a)
	}
	if len(positional) != 1 || outDir == "" {
		return "", "", false
	}
	return positional[0], outDir, true
}
