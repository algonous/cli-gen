package codegen

import "github.com/autonous/cli-gen/internal/schema"

type Generator struct{}

func New() *Generator {
	return &Generator{}
}

func (g *Generator) Generate(_ *schema.SchemaSet, _ string) error {
	return nil
}
