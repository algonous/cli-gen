package codegen

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"strings"
	"text/template"

	"github.com/autonous/cli-gen/internal/schema"
)

type Generator struct{}

func New() *Generator {
	return &Generator{}
}

func (g *Generator) Generate(_ *schema.SchemaSet, _ string) error {
	return nil
}

type MainTemplateData struct {
	CliName        string
	CliDescription string
	Actions        []string
}

//go:embed templates/main.go.tmpl
var mainTemplate string

func RenderMain(set *schema.SchemaSet) (string, error) {
	actions := make([]string, 0, len(set.Actions))
	for _, a := range set.Actions {
		actions = append(actions, a.Name)
	}

	tpl, err := template.New("main.go.tmpl").Funcs(template.FuncMap{
		"handlerName": toHandlerName,
	}).Parse(mainTemplate)
	if err != nil {
		return "", fmt.Errorf("parse main template: %w", err)
	}

	data := MainTemplateData{
		CliName:        set.CLI.Cli.Name,
		CliDescription: set.CLI.Cli.Description,
		Actions:        actions,
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute main template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("format rendered main: %w", err)
	}
	return string(formatted), nil
}

func toHandlerName(action string) string {
	parts := strings.Split(action, "-")
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}
