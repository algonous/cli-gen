package codegen

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"net/http"
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

//go:embed templates/request_builder.go.tmpl
var requestBuilderTemplate string

//go:embed templates/body_builder.go.tmpl
var bodyBuilderTemplate string

//go:embed templates/executor.go.tmpl
var executorTemplate string

//go:embed templates/custom_dispatcher.go.tmpl
var customDispatcherTemplate string

//go:embed templates/custom_bindings.go.tmpl
var customBindingsTemplate string

//go:embed templates/secrets.go.tmpl
var secretsTemplate string

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

type RequestBuilderData struct {
	HandlerSuffix string
	Method        string
	PathLiteral   string
	BaseURLEnv    string
	QueryLines    []string
	HeaderLines   []string
	PathArgLines  []string
}

func RenderRequestBuilder(cli *schema.CliFile, action *schema.ActionFile) (string, error) {
	data := RequestBuilderData{
		HandlerSuffix: toHandlerName(action.Name),
		Method:        action.Request.Method,
		PathLiteral:   action.Request.Path,
		BaseURLEnv:    cli.Cli.Runtime.BaseURL.FromEnv,
	}
	envSet := map[string]schema.EnvEntry{}
	for _, env := range cli.Cli.Runtime.Env {
		envSet[env.Name] = env
	}
	argSet := map[string]schema.ArgDef{}
	for _, arg := range action.Args {
		argSet[arg.Name] = arg
	}

	for _, ph := range extractDirectPlaceholders(action.Request.Path) {
		if kind, name, ok := parseInlinePlaceholder(ph); ok && kind == "arg" {
			field := toFieldName(name)
			data.PathArgLines = append(data.PathArgLines, fmt.Sprintf("path = strings.ReplaceAll(path, %q, url.PathEscape(fmt.Sprintf(\"%%v\", parsed.%s)))", ph, field))
		}
	}

	for _, q := range action.Request.Query {
		data.QueryLines = append(data.QueryLines, queryLine(q, argSet, envSet)...)
	}
	for _, h := range action.Request.Headers {
		data.HeaderLines = append(data.HeaderLines, headerLine(h, argSet, envSet)...)
	}

	return renderGoTemplate("request_builder.go.tmpl", requestBuilderTemplate, data, nil)
}

type BodyBuilderData struct {
	HandlerSuffix      string
	Mode               string
	RawJSONArgField    string
	TemplateAssignExpr []string
}

func RenderBodyBuilder(action *schema.ActionFile) (string, error) {
	data := BodyBuilderData{
		HandlerSuffix: toHandlerName(action.Name),
	}
	if action.Request.Body != nil {
		data.Mode = action.Request.Body.Mode
	}
	argSet := map[string]schema.ArgDef{}
	for _, arg := range action.Args {
		argSet[arg.Name] = arg
	}
	if action.Request.Body != nil && action.Request.Body.Mode == "raw_json_arg" {
		data.RawJSONArgField = toFieldName(action.Request.Body.Arg)
	}
	if action.Request.Body != nil && action.Request.Body.Mode == "template" {
		data.TemplateAssignExpr = buildTemplateAssignments(action.Request.Body.Template, argSet, "payload")
	}
	return renderGoTemplate("body_builder.go.tmpl", bodyBuilderTemplate, data, nil)
}

type ExecutorTemplateData struct {
	HandlerSuffix   string
	SuccessStatuses []int
}

func RenderExecutor(action *schema.ActionFile) (string, error) {
	data := ExecutorTemplateData{
		HandlerSuffix:   toHandlerName(action.Name),
		SuccessStatuses: action.Response.SuccessStatus,
	}
	return renderGoTemplate("executor.go.tmpl", executorTemplate, data, nil)
}

type CustomActionData struct {
	ActionName     string
	HandlerSuffix  string
	ArgsTypeName   string
	BindingFunc    string
	PlaceholderMsg string
}

func RenderCustomDispatcher(actionName string) (string, error) {
	data := CustomActionData{
		ActionName:     actionName,
		HandlerSuffix:  toHandlerName(actionName),
		ArgsTypeName:   toHandlerName(actionName) + "Args",
		BindingFunc:    "customBinding" + toHandlerName(actionName),
		PlaceholderMsg: fmt.Sprintf("PLACEHOLDER: %s requires a custom binding", actionName),
	}
	return renderGoTemplate("custom_dispatcher.go.tmpl", customDispatcherTemplate, data, nil)
}

func RenderCustomBindingStub(actionName string) (string, error) {
	data := CustomActionData{
		ActionName:     actionName,
		HandlerSuffix:  toHandlerName(actionName),
		ArgsTypeName:   toHandlerName(actionName) + "Args",
		BindingFunc:    "customBinding" + toHandlerName(actionName),
		PlaceholderMsg: fmt.Sprintf("PLACEHOLDER: %s requires a custom binding", actionName),
	}
	return renderGoTemplate("custom_bindings.go.tmpl", customBindingsTemplate, data, nil)
}

type SecretTemplateData struct {
	SecretEnvNames []string
}

func RenderSecretsHelpers(cli *schema.CliFile) (string, error) {
	names := []string{}
	for _, env := range cli.Cli.Runtime.Env {
		if env.Secret {
			names = append(names, env.Name)
		}
	}
	return renderGoTemplate("secrets.go.tmpl", secretsTemplate, SecretTemplateData{SecretEnvNames: names}, nil)
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
	t := template.New(name)
	if funcs != nil {
		t = t.Funcs(funcs)
	}
	tpl, err := t.Parse(source)
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

func extractDirectPlaceholders(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	start := -1
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '{':
			start = i
		case '}':
			if start >= 0 {
				out = append(out, value[start:i+1])
				start = -1
			}
		}
	}
	return out
}

func parseInlinePlaceholder(ph string) (kind, name string, ok bool) {
	if len(ph) < 2 || ph[0] != '{' || ph[len(ph)-1] != '}' {
		return "", "", false
	}
	inner := ph[1 : len(ph)-1]
	parts := strings.Split(inner, ".")
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	if parts[0] != "arg" && parts[0] != "env" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func queryLine(q schema.QueryParam, argSet map[string]schema.ArgDef, envSet map[string]schema.EnvEntry) []string {
	if s, ok := q.Value.(string); ok {
		if kind, name, ok := parseInlinePlaceholder(s); ok {
			if kind == "arg" {
				arg := argSet[name]
				field := toFieldName(name)
				return queryLineForArg(q.Name, field, arg, q.ArrayFormat)
			}
			if kind == "env" {
				env := envSet[name]
				return queryLineForEnv(q.Name, name, env.Required)
			}
		}
	}
	return []string{fmt.Sprintf("query.Add(%q, %q)", q.Name, fmt.Sprintf("%v", q.Value))}
}

func queryLineForArg(name, field string, arg schema.ArgDef, arrayFormat string) []string {
	switch arg.Type {
	case "array":
		switch arrayFormat {
		case "csv":
			return []string{
				fmt.Sprintf("if len(parsed.%s) > 0 { query.Add(%q, strings.Join(parsed.%s, \",\")) }", field, name, field),
			}
		case "brackets":
			return []string{
				fmt.Sprintf("for _, v := range parsed.%s { query.Add(%q, v) }", field, name+"[]"),
			}
		default:
			return []string{
				fmt.Sprintf("for _, v := range parsed.%s { query.Add(%q, v) }", field, name),
			}
		}
	case "number":
		if arg.Required {
			return []string{fmt.Sprintf("query.Add(%q, strconv.FormatFloat(parsed.%s, 'f', -1, 64))", name, field)}
		}
		return []string{fmt.Sprintf("if parsed.%s != 0 { query.Add(%q, strconv.FormatFloat(parsed.%s, 'f', -1, 64)) }", field, name, field)}
	case "boolean":
		if arg.Required {
			return []string{fmt.Sprintf("query.Add(%q, strconv.FormatBool(parsed.%s))", name, field)}
		}
		return []string{fmt.Sprintf("if parsed.%s { query.Add(%q, strconv.FormatBool(parsed.%s)) }", field, name, field)}
	default:
		if arg.Required {
			return []string{fmt.Sprintf("query.Add(%q, parsed.%s)", name, field)}
		}
		return []string{fmt.Sprintf("if parsed.%s != \"\" { query.Add(%q, parsed.%s) }", field, name, field)}
	}
}

func queryLineForEnv(name, envName string, required bool) []string {
	if required {
		return []string{fmt.Sprintf("query.Add(%q, envs[%q])", name, envName)}
	}
	return []string{fmt.Sprintf("if v, ok := envs[%q]; ok && v != \"\" { query.Add(%q, v) }", envName, name)}
}

func headerLine(h schema.Header, argSet map[string]schema.ArgDef, envSet map[string]schema.EnvEntry) []string {
	if s, ok := h.Value.(string); ok {
		if kind, name, ok := parseInlinePlaceholder(s); ok {
			if kind == "arg" {
				arg := argSet[name]
				field := toFieldName(name)
				if arg.Required {
					return []string{fmt.Sprintf("req.Header.Set(%q, fmt.Sprintf(\"%%v\", parsed.%s))", h.Name, field)}
				}
				switch arg.Type {
				case "number":
					return []string{fmt.Sprintf("if parsed.%s != 0 { req.Header.Set(%q, fmt.Sprintf(\"%%v\", parsed.%s)) }", field, h.Name, field)}
				case "boolean":
					return []string{fmt.Sprintf("if parsed.%s { req.Header.Set(%q, fmt.Sprintf(\"%%v\", parsed.%s)) }", field, h.Name, field)}
				case "array":
					return []string{fmt.Sprintf("if len(parsed.%s) > 0 { req.Header.Set(%q, strings.Join(parsed.%s, \",\")) }", field, h.Name, field)}
				default:
					return []string{fmt.Sprintf("if parsed.%s != \"\" { req.Header.Set(%q, parsed.%s) }", field, h.Name, field)}
				}
			}
			if kind == "env" {
				env := envSet[name]
				if env.Required {
					return []string{fmt.Sprintf("req.Header.Set(%q, envs[%q])", h.Name, name)}
				}
				return []string{fmt.Sprintf("if v, ok := envs[%q]; ok && v != \"\" { req.Header.Set(%q, v) }", name, h.Name)}
			}
		}
	}
	return []string{fmt.Sprintf("req.Header.Set(%q, %q)", h.Name, fmt.Sprintf("%v", h.Value))}
}

func buildTemplateAssignments(template any, argSet map[string]schema.ArgDef, root string) []string {
	obj, ok := template.(map[string]any)
	if !ok {
		return nil
	}
	lines := []string{}
	for key, value := range obj {
		if s, ok := value.(string); ok {
			if kind, name, ok := parseInlinePlaceholder(s); ok && kind == "arg" {
				arg, exists := argSet[name]
				if !exists {
					continue
				}
				field := "parsed." + toFieldName(name)
				switch arg.Type {
				case "array":
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = []any{}", root, key))
						lines = append(lines, fmt.Sprintf("for _, v := range %s { %s[%q] = append(%s[%q].([]any), v) }", field, root, key, root, key))
					} else {
						lines = append(lines, fmt.Sprintf("if len(%s) > 0 {", field))
						lines = append(lines, fmt.Sprintf("  %s[%q] = []any{}", root, key))
						lines = append(lines, fmt.Sprintf("  for _, v := range %s { %s[%q] = append(%s[%q].([]any), v) }", field, root, key, root, key))
						lines = append(lines, "}")
					}
				case "boolean":
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = %s", root, key, field))
					} else {
						lines = append(lines, fmt.Sprintf("if %s { %s[%q] = %s }", field, root, key, field))
					}
				case "number":
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = %s", root, key, field))
					} else {
						lines = append(lines, fmt.Sprintf("if %s != 0 { %s[%q] = %s }", field, root, key, field))
					}
				default:
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = %s", root, key, field))
					} else {
						lines = append(lines, fmt.Sprintf("if %s != \"\" { %s[%q] = %s }", field, root, key, field))
					}
				}
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("%s[%q] = %#v", root, key, value))
	}
	return lines
}

type Envelope struct {
	OK       bool   `json:"ok"`
	Status   int    `json:"status"`
	Body     any    `json:"body,omitempty"`
	BodyText string `json:"body_text,omitempty"`
}

func BuildEnvelope(successStatuses []int, resp *http.Response, body []byte) (Envelope, error) {
	ok := false
	for _, code := range successStatuses {
		if resp.StatusCode == code {
			ok = true
			break
		}
	}
	env := Envelope{OK: ok, Status: resp.StatusCode}
	if len(body) == 0 {
		env.BodyText = ""
		return env, nil
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err == nil {
		env.Body = parsed
		return env, nil
	}
	env.BodyText = string(body)
	return env, nil
}
