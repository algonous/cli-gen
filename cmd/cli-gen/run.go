package main

import (
	"fmt"
	"os"

	"github.com/autonous/cli-gen/internal/codegen"
	"github.com/autonous/cli-gen/internal/schema"
)

func RunGenerator(schemaDir, outDir string) error {
	set, err := schema.LoadSchemaDir(schemaDir)
	if err != nil {
		return err
	}
	if err := schema.CheckDuplicateActions(set); err != nil {
		return err
	}

	var valErrs []schema.ValidationError
	valErrs = append(valErrs, schema.ValidateCLIFile(set.CLI)...)
	for _, action := range set.Actions {
		valErrs = append(valErrs, schema.ValidateActionFile(action)...)
		valErrs = append(valErrs, schema.ValidatePlaceholders(set.CLI, action)...)
		valErrs = append(valErrs, schema.DetectUnusedArgs(action)...)
		valErrs = append(valErrs, schema.ValidateResponseHints(action)...)
	}
	if len(valErrs) > 0 {
		for _, ve := range valErrs {
			fmt.Fprintf(os.Stderr, "%s: %s: %s\n", ve.File, ve.Field, ve.Message)
		}
		return fmt.Errorf("schema validation failed")
	}

	gen := codegen.New()
	if err := gen.Generate(set, outDir); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Generated %d actions -> %s\n", len(set.Actions), outDir)
	return nil
}
