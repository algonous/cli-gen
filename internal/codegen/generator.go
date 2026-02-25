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

// ---------------------------------------------------------------------------
// Embedded templates
// ---------------------------------------------------------------------------

//go:embed templates/main.go.tmpl
var mainTemplate string

//go:embed templates/skill.go.tmpl
var skillTemplate string

//go:embed templates/internal_runtime.go.tmpl
var internalRuntimeTemplate string

//go:embed templates/generated_action.go.tmpl
var generatedActionTemplate string

//go:embed templates/custom_action.go.tmpl
var customActionTemplate string

// ---------------------------------------------------------------------------
// RenderMain
// ---------------------------------------------------------------------------

type MainTemplateData struct {
	ModuleName      string
	CliName         string
	CliDescription  string
	GeneratedActions []string
	CustomActions   []string
	AllActions      []string
	HasGenerated    bool
	HasCustom       bool
}

func RenderMain(set *schema.SchemaSet) (string, error) {
	var generated, custom, all []string
	for _, a := range set.Actions {
		all = append(all, a.Name)
		if a.Impl == "custom" {
			custom = append(custom, a.Name)
		} else {
			generated = append(generated, a.Name)
		}
	}
	data := MainTemplateData{
		ModuleName:      set.CLI.Cli.Name,
		CliName:         set.CLI.Cli.Name,
		CliDescription:  set.CLI.Cli.Description,
		GeneratedActions: generated,
		CustomActions:   custom,
		AllActions:      all,
		HasGenerated:    len(generated) > 0,
		HasCustom:       len(custom) > 0,
	}
	return renderGoTemplate("main.go.tmpl", mainTemplate, data, template.FuncMap{
		"handlerName": toHandlerName,
	})
}

// ---------------------------------------------------------------------------
// RenderSkillHandler
// ---------------------------------------------------------------------------

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
	ModuleName         string
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
		ModuleName:         set.CLI.Cli.Name,
		CliName:            set.CLI.Cli.Name,
		SkillName:          set.CLI.Cli.Name + "-skill",
		SkillDescription:   buildSkillDescription(set.CLI.Cli.Name, actions),
		TriggerHints:       buildSkillTriggerHints(actions),
		PrimaryExampleList: buildPrimaryExamples(set.CLI.Cli.Name, actions),
		Actions:            actions,
	}, nil)
}

// ---------------------------------------------------------------------------
// RenderInternalRuntime
// ---------------------------------------------------------------------------

type InternalRuntimeData struct {
	SecretEnvNames []string
}

func RenderInternalRuntime(cli *schema.CliFile) (string, error) {
	var secrets []string
	for _, env := range cli.Cli.Runtime.Env {
		if env.Secret {
			secrets = append(secrets, env.Name)
		}
	}
	return renderGoTemplate("internal_runtime.go.tmpl", internalRuntimeTemplate,
		InternalRuntimeData{SecretEnvNames: secrets}, nil)
}

// ---------------------------------------------------------------------------
// RenderGeneratedAction
// ---------------------------------------------------------------------------

type GeneratedActionData struct {
	ModuleName         string
	ActionName         string
	HandlerSuffix      string
	Args               []schema.ArgDef
	RequiredFlags      []string
	Method             string
	PathLiteral        string
	BaseURLEnv         string
	QueryLines         []string
	HeaderLines        []string
	PathArgLines       []string
	HasBody            bool
	BodyMode           string
	RawJSONArgField    string
	TemplateAssignExpr []string
	EnvNames           []string
	RequiredEnvs       []string
	SuccessStatuses    []int
	NeedsStrconv       bool
	NeedsStrings       bool
	NeedsJSON          bool
}

func RenderGeneratedAction(moduleName string, cli *schema.CliFile, action *schema.ActionFile) (string, error) {
	envSet := map[string]schema.EnvEntry{}
	for _, env := range cli.Cli.Runtime.Env {
		envSet[env.Name] = env
	}
	argSet := map[string]schema.ArgDef{}
	for _, arg := range action.Args {
		argSet[arg.Name] = arg
	}

	var requiredFlags, envNames, requiredEnvs []string
	for _, arg := range action.Args {
		if arg.Required {
			requiredFlags = append(requiredFlags, arg.Name)
		}
	}
	for _, env := range cli.Cli.Runtime.Env {
		envNames = append(envNames, env.Name)
		if env.Required {
			requiredEnvs = append(requiredEnvs, env.Name)
		}
	}

	// Path arg substitution lines
	var pathArgLines []string
	for _, ph := range extractDirectPlaceholders(action.Request.Path) {
		if kind, name, ok := parseInlinePlaceholder(ph); ok && kind == "arg" {
			field := toFieldName(name)
			pathArgLines = append(pathArgLines,
				fmt.Sprintf("path = strings.ReplaceAll(path, %q, url.PathEscape(fmt.Sprintf(\"%%v\", parsed.%s)))", ph, field))
		}
	}

	// Query and header lines
	var queryLines []string
	for _, q := range action.Request.Query {
		queryLines = append(queryLines, queryLine(q, argSet, envSet)...)
	}
	var headerLines []string
	for _, h := range action.Request.Headers {
		headerLines = append(headerLines, headerLine(h, argSet, envSet)...)
	}
	if cli.Cli.Runtime.Auth != nil {
		headerLines = append(headerLines, authHeaderLines(cli.Cli.Runtime.Auth, argSet, envSet)...)
	}

	// Body
	hasBody := action.Request.Body != nil && action.Request.Body.Mode != ""
	bodyMode := ""
	rawJSONArgField := ""
	var templateAssignExpr []string
	if action.Request.Body != nil {
		bodyMode = action.Request.Body.Mode
		if bodyMode == "raw_json_arg" {
			rawJSONArgField = toFieldName(action.Request.Body.Arg)
		}
		if bodyMode == "template" {
			templateAssignExpr = buildTemplateAssignments(action.Request.Body.Template, argSet, "payload")
		}
	}

	// Compute import flags by scanning generated lines
	allLines := append(append(queryLines, headerLines...), pathArgLines...)
	needsStrconv := containsStr(allLines, "strconv.")
	needsStrings := true // template always uses strings.HasPrefix + strings.ReplaceAll in arg parsing
	needsJSON := hasBody

	data := GeneratedActionData{
		ModuleName:         moduleName,
		ActionName:         action.Name,
		HandlerSuffix:      toHandlerName(action.Name),
		Args:               action.Args,
		RequiredFlags:      requiredFlags,
		Method:             action.Request.Method,
		PathLiteral:        action.Request.Path,
		BaseURLEnv:         cli.Cli.Runtime.BaseURL.FromEnv,
		QueryLines:         queryLines,
		HeaderLines:        headerLines,
		PathArgLines:       pathArgLines,
		HasBody:            hasBody,
		BodyMode:           bodyMode,
		RawJSONArgField:    rawJSONArgField,
		TemplateAssignExpr: templateAssignExpr,
		EnvNames:           envNames,
		RequiredEnvs:       requiredEnvs,
		SuccessStatuses:    action.Response.SuccessStatus,
		NeedsStrconv:       needsStrconv,
		NeedsStrings:       needsStrings,
		NeedsJSON:          needsJSON,
	}
	return renderGoTemplate("generated_action.go.tmpl", generatedActionTemplate, data, template.FuncMap{
		"fieldName": toFieldName,
		"varName":   toVarName,
		"flagType":  flagMethodForType,
		"isArray": func(t string) bool {
			return t == "array"
		},
	})
}

func containsStr(lines []string, substr string) bool {
	for _, l := range lines {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// RenderCustomAction
// ---------------------------------------------------------------------------

type CustomActionData struct {
	ModuleName    string
	ActionName    string
	HandlerSuffix string
}

func RenderCustomAction(moduleName string, action *schema.ActionFile) (string, error) {
	data := CustomActionData{
		ModuleName:    moduleName,
		ActionName:    action.Name,
		HandlerSuffix: toHandlerName(action.Name),
	}
	return renderGoTemplate("custom_action.go.tmpl", customActionTemplate, data, nil)
}

// ---------------------------------------------------------------------------
// Generate
// ---------------------------------------------------------------------------

func (g *Generator) Generate(set *schema.SchemaSet, outDir string) error {
	moduleName := set.CLI.Cli.Name

	// Create top-level and sub-directories
	internalDir := filepath.Join(outDir, "internal")
	generatedDir := filepath.Join(outDir, "generated")
	customDir := filepath.Join(outDir, "custom")
	for _, dir := range []string{outDir, internalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Determine whether we need generated/ and custom/ dirs
	var hasGenerated, hasCustom bool
	for _, a := range set.Actions {
		if a.Impl == "custom" {
			hasCustom = true
		} else {
			hasGenerated = true
		}
	}
	if hasGenerated {
		if err := os.MkdirAll(generatedDir, 0o755); err != nil {
			return fmt.Errorf("mkdir generated: %w", err)
		}
	}
	if hasCustom {
		if err := os.MkdirAll(customDir, 0o755); err != nil {
			return fmt.Errorf("mkdir custom: %w", err)
		}
	}

	// main.go
	mainSrc, err := RenderMain(set)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		return err
	}

	// skill.go
	skillSrc, err := RenderSkillHandler(set)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "skill.go"), []byte(skillSrc), 0o644); err != nil {
		return err
	}

	// internal/runtime.go
	runtimeSrc, err := RenderInternalRuntime(set.CLI)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(internalDir, "runtime.go"), []byte(runtimeSrc), 0o644); err != nil {
		return err
	}

	// Per-action files
	for _, action := range set.Actions {
		fileName := strings.ToLower(strings.ReplaceAll(action.Name, "-", "_")) + ".go"
		if action.Impl == "custom" {
			src, err := RenderCustomAction(moduleName, action)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(customDir, fileName), []byte(src), 0o644); err != nil {
				return err
			}
		} else {
			src, err := RenderGeneratedAction(moduleName, set.CLI, action)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(generatedDir, fileName), []byte(src), 0o644); err != nil {
				return err
			}
		}
	}

	// go.mod
	goMod := fmt.Sprintf("module %s\n\ngo 1.25.0\n", moduleName)
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Name conversion helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Template rendering
// ---------------------------------------------------------------------------

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
		return "", fmt.Errorf("format rendered %s: %w\n--- source ---\n%s", name, err, buf.String())
	}
	return string(formatted), nil
}

// ---------------------------------------------------------------------------
// Request building helpers (unchanged from previous implementation)
// ---------------------------------------------------------------------------

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

func buildTemplateAssignments(tmpl any, argSet map[string]schema.ArgDef, root string) []string {
	obj, ok := tmpl.(map[string]any)
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

// ---------------------------------------------------------------------------
// Skill helpers (unchanged)
// ---------------------------------------------------------------------------

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
