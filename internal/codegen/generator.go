package codegen

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/autonous/cli-gen/internal/schema"
)

type Generator struct{}

func New() *Generator {
	return &Generator{}
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

//go:embed templates/logfile.go.tmpl
var logfileTemplate string

//go:embed templates/skill.go.tmpl
var skillTemplate string

//go:embed templates/action_handler.go.tmpl
var actionHandlerTemplate string

//go:embed templates/runtime_helpers.go.tmpl
var runtimeHelpersTemplate string

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
	if cli.Cli.Runtime.Auth != nil {
		data.HeaderLines = append(data.HeaderLines, authHeaderLines(cli.Cli.Runtime.Auth, argSet, envSet)...)
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

func RenderLogFileHelper() (string, error) {
	return renderGoTemplate("logfile.go.tmpl", logfileTemplate, nil, nil)
}

type SkillActionData struct {
	Name        string
	Description string
	Args        []schema.ArgDef
	Method      string
	Path        string
	References  []string
	Examples    []string
}

type SkillTemplateData struct {
	CliName            string
	SkillName          string
	SkillDescription   string
	TriggerHints       string
	Actions            []SkillActionData
	PrimaryExampleList []string
}

func RenderSkillHandler(set *schema.SchemaSet) (string, error) {
	actions := make([]SkillActionData, 0, len(set.Actions))
	for _, a := range set.Actions {
		actions = append(actions, SkillActionData{
			Name:        a.Name,
			Description: a.Description,
			Args:        a.Args,
			Method:      a.Request.Method,
			Path:        a.Request.Path,
			References:  a.References,
			Examples:    buildSkillExamples(set.CLI.Cli.Name, a),
		})
	}
	return renderGoTemplate("skill.go.tmpl", skillTemplate, SkillTemplateData{
		CliName:            set.CLI.Cli.Name,
		SkillName:          set.CLI.Cli.Name + "-skill",
		SkillDescription:   buildSkillDescription(set.CLI.Cli.Name, actions),
		TriggerHints:       buildSkillTriggerHints(actions),
		PrimaryExampleList: buildPrimaryExamples(set.CLI.Cli.Name, actions),
		Actions:            actions,
	}, nil)
}

func buildSkillExamples(cliName string, action *schema.ActionFile) []string {
	baseArgs := map[string][]string{}
	for _, arg := range action.Args {
		baseArgs[arg.Name] = sampleArgValues(arg.Name, arg.Type)
	}

	requiredOnly := []string{}
	for _, arg := range action.Args {
		if arg.Required {
			requiredOnly = append(requiredOnly, arg.Name)
		}
	}

	examples := []string{buildSkillCommand(cliName, action.Name, requiredOnly, baseArgs)}

	// For repository contents endpoints, include both root listing and file read examples.
	if strings.Contains(action.Request.Path, "/contents") && hasArg(action.Args, "owner") && hasArg(action.Args, "repo") {
		if strings.Contains(action.Name, "list") && hasArg(action.Args, "path") {
			overrides := copyArgValues(baseArgs)
			overrides["path"] = []string{""}
			examples = appendUniqueExample(examples, buildSkillCommand(cliName, action.Name, []string{"owner", "repo", "path"}, overrides))
		}
		if hasOptionalArg(action.Args, "path") {
			examples = appendUniqueExample(examples, buildSkillCommand(cliName, action.Name, []string{"owner", "repo"}, baseArgs))
		}
		if hasArg(action.Args, "path") {
			examples = appendUniqueExample(examples, buildSkillCommand(cliName, action.Name, []string{"owner", "repo", "path"}, baseArgs))
		}
	}

	// For issue creation endpoints, include a complete issue sample with title/body.
	if strings.EqualFold(action.Request.Method, "POST") && strings.Contains(action.Request.Path, "/issues") {
		argNames := []string{"owner", "repo", "title"}
		if hasArg(action.Args, "body") {
			argNames = append(argNames, "body")
		}
		examples = appendUniqueExample(examples, buildSkillCommand(cliName, action.Name, argNames, baseArgs))
	}

	return examples
}

func appendUniqueExample(examples []string, candidate string) []string {
	if candidate == "" {
		return examples
	}
	for _, e := range examples {
		if e == candidate {
			return examples
		}
	}
	return append(examples, candidate)
}

func copyArgValues(values map[string][]string) map[string][]string {
	out := map[string][]string{}
	for k, v := range values {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func buildSkillCommand(cliName, actionName string, argOrder []string, values map[string][]string) string {
	parts := []string{cliName, actionName}
	for _, argName := range argOrder {
		argValues := values[argName]
		if len(argValues) == 0 {
			continue
		}
		for _, v := range argValues {
			parts = append(parts, "--"+argName, shellQuote(v))
		}
	}
	return strings.Join(parts, " ")
}

func hasArg(args []schema.ArgDef, name string) bool {
	for _, arg := range args {
		if arg.Name == name {
			return true
		}
	}
	return false
}

func hasOptionalArg(args []schema.ArgDef, name string) bool {
	for _, arg := range args {
		if arg.Name == name {
			return !arg.Required
		}
	}
	return false
}

func sampleArgValues(name, typ string) []string {
	switch name {
	case "owner":
		return []string{"algonous"}
	case "repo":
		return []string{"cli-gen"}
	case "path":
		return []string{"README.md"}
	case "title":
		return []string{"Automated issue from generated CLI"}
	case "body":
		return []string{"This issue is created by the generated CLI skill example."}
	case "labels":
		return []string{"bug", "api"}
	case "state":
		return []string{"open"}
	case "ref":
		return []string{"main"}
	case "page":
		return []string{"1"}
	case "per_page":
		return []string{"30"}
	}

	switch typ {
	case "array":
		return []string{"item1", "item2"}
	case "number":
		return []string{"1"}
	case "boolean":
		return []string{"true"}
	case "json":
		return []string{`{"example":"value"}`}
	default:
		return []string{"example"}
	}
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func buildSkillDescription(cliName string, actions []SkillActionData) string {
	actionNames := make([]string, 0, len(actions))
	for _, a := range actions {
		actionNames = append(actionNames, a.Name)
	}
	return fmt.Sprintf("Use this skill to operate `%s` for GitHub REST tasks. Trigger when users ask to run these actions: %s.", cliName, strings.Join(actionNames, ", "))
}

func buildSkillTriggerHints(actions []SkillActionData) string {
	hints := []string{}
	for _, a := range actions {
		if strings.Contains(a.Name, "list") && strings.Contains(a.Name, "file") {
			hints = append(hints, "list files in a repository")
		}
		if strings.Contains(a.Name, "read") && strings.Contains(a.Name, "file") {
			hints = append(hints, "read file contents from a repository")
		}
		if strings.Contains(a.Name, "create-issue") || (strings.Contains(a.Name, "issue") && strings.Contains(a.Name, "create")) {
			hints = append(hints, "create an issue")
		}
	}
	if len(hints) == 0 {
		return "call GitHub API actions with deterministic CLI commands"
	}
	return strings.Join(hints, "; ")
}

func buildPrimaryExamples(cliName string, actions []SkillActionData) []string {
	examples := []string{}
	for _, a := range actions {
		switch {
		case strings.Contains(a.Name, "list") && strings.Contains(a.Name, "file"):
			examples = appendUniqueExample(examples, fmt.Sprintf("%s %s --owner 'algonous' --repo 'cli-gen' --path ''", cliName, a.Name))
		case strings.Contains(a.Name, "read") && strings.Contains(a.Name, "file"):
			examples = appendUniqueExample(examples, fmt.Sprintf("%s %s --owner 'algonous' --repo 'cli-gen' --path 'README.md'", cliName, a.Name))
		case strings.Contains(a.Name, "issue") && strings.Contains(a.Name, "create"):
			examples = appendUniqueExample(examples, fmt.Sprintf("%s %s --owner 'algonous' --repo 'cli-gen' --title 'Skill test issue' --body 'Created during SKILL.md validation run.'", cliName, a.Name))
		}
	}
	return examples
}

type ActionHandlerData struct {
	HandlerSuffix string
	HasBody       bool
	EnvNames      []string
	RequiredEnvs  []string
}

func RenderActionHandler(cli *schema.CliFile, action *schema.ActionFile) (string, error) {
	envNames := make([]string, 0, len(cli.Cli.Runtime.Env))
	required := make([]string, 0, len(cli.Cli.Runtime.Env))
	for _, e := range cli.Cli.Runtime.Env {
		envNames = append(envNames, e.Name)
		if e.Required {
			required = append(required, e.Name)
		}
	}
	data := ActionHandlerData{
		HandlerSuffix: toHandlerName(action.Name),
		HasBody:       action.Request.Body != nil && action.Request.Body.Mode != "",
		EnvNames:      envNames,
		RequiredEnvs:  required,
	}
	return renderGoTemplate("action_handler.go.tmpl", actionHandlerTemplate, data, nil)
}

func RenderRuntimeHelpers() (string, error) {
	return renderGoTemplate("runtime_helpers.go.tmpl", runtimeHelpersTemplate, nil, nil)
}

func (g *Generator) Generate(set *schema.SchemaSet, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir output dir: %w", err)
	}

	mainSrc, err := RenderMain(set)
	if err != nil {
		return err
	}
	skillSrc, err := RenderSkillHandler(set)
	if err != nil {
		return err
	}
	logSrc, err := RenderLogFileHelper()
	if err != nil {
		return err
	}
	secretSrc, err := RenderSecretsHelpers(set.CLI)
	if err != nil {
		return err
	}
	runtimeSrc, err := RenderRuntimeHelpers()
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(outDir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "skill.go"), []byte(skillSrc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "logfile.go"), []byte(logSrc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "secrets.go"), []byte(secretSrc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "runtime_helpers.go"), []byte(runtimeSrc), 0o644); err != nil {
		return err
	}

	for _, action := range set.Actions {
		suffix := strings.ToLower(strings.ReplaceAll(action.Name, "-", "_"))
		if action.Impl == "custom" {
			dispatcher, err := RenderCustomDispatcher(action.Name)
			if err != nil {
				return err
			}
			parser, err := RenderArgParser(action)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_handler.go"), []byte(dispatcher), 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_arg_parser.go"), []byte(parser), 0o644); err != nil {
				return err
			}
			stub, err := RenderCustomBindingStub(action.Name)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_custom_binding.go"), []byte(stub), 0o644); err != nil {
				return err
			}
			continue
		}

		handler, err := RenderActionHandler(set.CLI, action)
		if err != nil {
			return err
		}
		parser, err := RenderArgParser(action)
		if err != nil {
			return err
		}
		reqBuilder, err := RenderRequestBuilder(set.CLI, action)
		if err != nil {
			return err
		}
		bodyBuilder, err := RenderBodyBuilder(action)
		if err != nil {
			return err
		}
		executor, err := RenderExecutor(action)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_handler.go"), []byte(handler), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_arg_parser.go"), []byte(parser), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_request.go"), []byte(reqBuilder), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_body.go"), []byte(bodyBuilder), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "action_"+suffix+"_exec.go"), []byte(executor), 0o644); err != nil {
			return err
		}
	}
	goMod := fmt.Sprintf("module %s\n\ngo 1.25.0\n", set.CLI.Cli.Name)
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}
	return nil
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
				return queryLineForArg(q.Name, name, field, arg, q.ArrayFormat)
			}
			if kind == "env" {
				env := envSet[name]
				return queryLineForEnv(q.Name, name, env.Required)
			}
		}
	}
	return []string{fmt.Sprintf("query.Add(%q, %q)", q.Name, fmt.Sprintf("%v", q.Value))}
}

func queryLineForArg(name, argName, field string, arg schema.ArgDef, arrayFormat string) []string {
	present := fmt.Sprintf("parsed.Provided[%q]", argName)
	switch arg.Type {
	case "array":
		switch arrayFormat {
		case "csv":
			return []string{
				fmt.Sprintf("if %s { query.Add(%q, strings.Join(parsed.%s, \",\")) }", present, name, field),
			}
		case "brackets":
			return []string{
				fmt.Sprintf("if %s { for _, v := range parsed.%s { query.Add(%q, v) } }", present, field, name+"[]"),
			}
		default:
			return []string{
				fmt.Sprintf("if %s { for _, v := range parsed.%s { query.Add(%q, v) } }", present, field, name),
			}
		}
	case "number":
		if arg.Required {
			return []string{fmt.Sprintf("query.Add(%q, strconv.FormatFloat(parsed.%s, 'f', -1, 64))", name, field)}
		}
		return []string{fmt.Sprintf("if %s { query.Add(%q, strconv.FormatFloat(parsed.%s, 'f', -1, 64)) }", present, name, field)}
	case "boolean":
		if arg.Required {
			return []string{fmt.Sprintf("query.Add(%q, strconv.FormatBool(parsed.%s))", name, field)}
		}
		return []string{fmt.Sprintf("if %s { query.Add(%q, strconv.FormatBool(parsed.%s)) }", present, name, field)}
	default:
		if arg.Required {
			return []string{fmt.Sprintf("query.Add(%q, parsed.%s)", name, field)}
		}
		return []string{fmt.Sprintf("if %s { query.Add(%q, parsed.%s) }", present, name, field)}
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
				present := fmt.Sprintf("parsed.Provided[%q]", name)
				switch arg.Type {
				case "number":
					return []string{fmt.Sprintf("if %s { req.Header.Set(%q, fmt.Sprintf(\"%%v\", parsed.%s)) }", present, h.Name, field)}
				case "boolean":
					return []string{fmt.Sprintf("if %s { req.Header.Set(%q, fmt.Sprintf(\"%%v\", parsed.%s)) }", present, h.Name, field)}
				case "array":
					return []string{fmt.Sprintf("if %s { req.Header.Set(%q, strings.Join(parsed.%s, \",\")) }", present, h.Name, field)}
				default:
					return []string{fmt.Sprintf("if %s { req.Header.Set(%q, parsed.%s) }", present, h.Name, field)}
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

func authHeaderLines(auth *schema.AuthDef, argSet map[string]schema.ArgDef, envSet map[string]schema.EnvEntry) []string {
	if auth == nil || auth.Header == "" || auth.Template == "" {
		return nil
	}
	expr := strconv.Quote(auth.Template)
	skips := []string{}
	for _, ph := range extractDirectPlaceholders(auth.Template) {
		kind, name, ok := parseInlinePlaceholder(ph)
		if !ok {
			continue
		}
		if kind == "env" {
			repl := fmt.Sprintf("envs[%q]", name)
			expr = fmt.Sprintf("strings.ReplaceAll(%s, %q, %s)", expr, ph, repl)
			if env, ok := envSet[name]; ok && !env.Required {
				skips = append(skips, fmt.Sprintf("envs[%q] == \"\"", name))
			}
		}
		if kind == "arg" {
			field := toFieldName(name)
			repl := fmt.Sprintf("fmt.Sprintf(\"%%v\", parsed.%s)", field)
			expr = fmt.Sprintf("strings.ReplaceAll(%s, %q, %s)", expr, ph, repl)
			if arg, ok := argSet[name]; ok && !arg.Required {
				skips = append(skips, fmt.Sprintf("!parsed.Provided[%q]", name))
			}
		}
	}
	if len(skips) == 0 {
		return []string{fmt.Sprintf("req.Header.Set(%q, %s)", auth.Header, expr)}
	}
	return []string{
		fmt.Sprintf("if !(%s) { req.Header.Set(%q, %s) }", strings.Join(skips, " || "), auth.Header, expr),
	}
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
						lines = append(lines, fmt.Sprintf("if parsed.Provided[%q] {", name))
						lines = append(lines, fmt.Sprintf("  %s[%q] = []any{}", root, key))
						lines = append(lines, fmt.Sprintf("  for _, v := range %s { %s[%q] = append(%s[%q].([]any), v) }", field, root, key, root, key))
						lines = append(lines, "}")
					}
				case "boolean":
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = %s", root, key, field))
					} else {
						lines = append(lines, fmt.Sprintf("if parsed.Provided[%q] { %s[%q] = %s }", name, root, key, field))
					}
				case "number":
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = %s", root, key, field))
					} else {
						lines = append(lines, fmt.Sprintf("if parsed.Provided[%q] { %s[%q] = %s }", name, root, key, field))
					}
				default:
					if arg.Required {
						lines = append(lines, fmt.Sprintf("%s[%q] = %s", root, key, field))
					} else {
						lines = append(lines, fmt.Sprintf("if parsed.Provided[%q] { %s[%q] = %s }", name, root, key, field))
					}
				}
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("%s[%q] = %#v", root, key, value))
	}
	return lines
}
