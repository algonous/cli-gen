package schema

import "fmt"

func DetectUnusedArgs(action *ActionFile) []ValidationError {
	file := "actions/" + action.Name + ".yaml"
	used := map[string]bool{}

	collectArgPlaceholders(action.Request.Path, used)
	for _, q := range action.Request.Query {
		if s, ok := stringFromAny(q.Value); ok {
			collectArgPlaceholders(s, used)
		}
	}
	for _, h := range action.Request.Headers {
		if s, ok := stringFromAny(h.Value); ok {
			collectArgPlaceholders(s, used)
		}
	}
	if action.Request.Body != nil {
		if action.Request.Body.Mode == "template" {
			values, _ := flattenTemplateStrings(action.Request.Body.Template, false)
			for _, v := range values {
				collectArgPlaceholders(v, used)
			}
		}
		if action.Request.Body.Arg != "" {
			used[action.Request.Body.Arg] = true
		}
	}

	var errs []ValidationError
	for _, arg := range action.Args {
		if !used[arg.Name] {
			errs = append(errs, newValidationError(file, "args", fmt.Sprintf("unused arg %q", arg.Name)))
		}
	}
	return errs
}

func collectArgPlaceholders(s string, used map[string]bool) {
	for _, ph := range extractPlaceholders(s) {
		kind, name, ok := parsePlaceholder(ph)
		if ok && kind == "arg" {
			used[name] = true
		}
	}
}
