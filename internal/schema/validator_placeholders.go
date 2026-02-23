package schema

import "fmt"

func ValidatePlaceholders(cli *CliFile, action *ActionFile) []ValidationError {
	file := "actions/" + action.Name + ".yaml"
	var errs []ValidationError

	argSet := map[string]ArgDef{}
	for _, arg := range action.Args {
		argSet[arg.Name] = arg
	}
	envSet := map[string]EnvEntry{}
	for _, env := range cli.Cli.Runtime.Env {
		envSet[env.Name] = env
	}

	pathErrs := placeholderErrors(file, "request.path", action.Request.Path, map[string]bool{"arg": true}, argSet, envSet)
	errs = append(errs, pathErrs...)

	for i, q := range action.Request.Query {
		if s, ok := stringFromAny(q.Value); ok {
			errs = append(errs, placeholderErrors(file, fmt.Sprintf("request.query[%d].value", i), s, map[string]bool{"arg": true, "env": true}, argSet, envSet)...)
		}
	}
	for i, h := range action.Request.Headers {
		if s, ok := stringFromAny(h.Value); ok {
			errs = append(errs, placeholderErrors(file, fmt.Sprintf("request.headers[%d].value", i), s, map[string]bool{"arg": true, "env": true}, argSet, envSet)...)
		}
	}

	if cli.Cli.Runtime.Auth != nil {
		errs = append(errs, placeholderErrors("cli.yaml", "cli.runtime.auth.template", cli.Cli.Runtime.Auth.Template, map[string]bool{"arg": true, "env": true}, argSet, envSet)...)
	}

	if action.Request.Body != nil && action.Request.Body.Mode == "template" {
		values, keys := flattenTemplateStrings(action.Request.Body.Template, true)
		for _, key := range keys {
			if len(extractPlaceholders(key)) > 0 {
				errs = append(errs, newValidationError(file, "request.body.template", fmt.Sprintf("placeholder in template key is not allowed: %q", key)))
			}
		}
		for _, v := range values {
			errs = append(errs, placeholderErrors(file, "request.body.template", v, map[string]bool{"arg": true, "env": true}, argSet, envSet)...)
			for _, ph := range extractPlaceholders(v) {
				kind, name, ok := parsePlaceholder(ph)
				if ok && kind == "arg" {
					if arg, exists := argSet[name]; exists && arg.Type == "json" {
						errs = append(errs, newValidationError(file, "request.body.template", fmt.Sprintf("arg %q with type json is not allowed in template placeholders", name)))
					}
				}
			}
		}
	}

	return errs
}
