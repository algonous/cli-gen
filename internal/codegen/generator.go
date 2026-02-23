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

//go:embed templates/arg_parser.go.tmpl
var argParserTemplate string

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

type ArgTemplateData struct {
	ActionName      string
	HandlerSuffix   string
	Args            []schema.ArgDef
	RequiredFlagSet []string
}

func RenderArgParser(action *schema.ActionFile) (string, error) {
	required := make([]string, 0)
	for _, arg := range action.Args {
		if arg.Required {
			required = append(required, arg.Name)
		}
	}
	data := ArgTemplateData{
		ActionName:      action.Name,
		HandlerSuffix:   toHandlerName(action.Name),
		Args:            action.Args,
		RequiredFlagSet: required,
	}
	out, err := renderGoTemplate("arg_parser.go.tmpl", argParserTemplate, data, template.FuncMap{
		"fieldName": toFieldName,
		"varName":   toVarName,
		"flagType":  flagMethodForType,
		"isArray": func(t string) bool {
			return t == "array"
		},
	})
	if err != nil {
		return "", err
	}
	return out, nil
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

func toFieldName(name string) string {
	return toHandlerName(name)
}

func toVarName(name string) string {
	field := toFieldName(name)
	if field == "" {
		return ""
	}
	return strings.ToLower(field[:1]) + field[1:]
}

func flagMethodForType(t string) string {
	switch t {
	case "number":
		return "Float64"
	case "boolean":
		return "Bool"
	default:
		return "String"
	}
}

func renderGoTemplate(name, source string, data any, funcs template.FuncMap) (string, error) {
	tpl, err := template.New(name).Funcs(funcs).Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("format rendered %s: %w", name, err)
	}
	return string(formatted), nil
}
