package schema

import (
	"fmt"
)

func ValidateActionFile(action *ActionFile) []ValidationError {
	var errs []ValidationError
	file := "actions/" + action.Name + ".yaml"

	if !argNamePattern.MatchString(action.Name) {
		errs = append(errs, newValidationError(file, "name", "must match ^[a-z][a-z0-9-]*$"))
	}
	if action.Impl != "generated" && action.Impl != "custom" {
		errs = append(errs, newValidationError(file, "impl", "must be generated or custom"))
	}
	if action.Description == "" {
		errs = append(errs, newValidationError(file, "description", "must be non-empty"))
	}
	switch action.Request.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		errs = append(errs, newValidationError(file, "request.method", "must be one of GET POST PUT PATCH DELETE"))
	}
	if action.Request.Path == "" {
		errs = append(errs, newValidationError(file, "request.path", "must be non-empty"))
	}

	argSet := map[string]ArgDef{}
	for i, arg := range action.Args {
		if !argNamePattern.MatchString(arg.Name) {
			errs = append(errs, newValidationError(file, fmt.Sprintf("args[%d].name", i), "must match ^[a-z][a-z0-9-]*$"))
		}
		switch arg.Type {
		case "string", "number", "boolean", "array", "json":
		default:
			errs = append(errs, newValidationError(file, fmt.Sprintf("args[%d].type", i), "invalid arg type"))
		}
		if arg.Help == "" {
			errs = append(errs, newValidationError(file, fmt.Sprintf("args[%d].help", i), "must be non-empty"))
		}
		argSet[arg.Name] = arg
	}

	if len(action.Response.SuccessStatus) == 0 {
		errs = append(errs, newValidationError(file, "response.success_status", "must be non-empty"))
	}

	if action.Request.Body != nil {
		if action.Request.Body.Mode != "" && action.Request.Body.Mode != "template" && action.Request.Body.Mode != "raw_json_arg" {
			errs = append(errs, newValidationError(file, "request.body.mode", "must be template or raw_json_arg"))
		}
		if action.Request.Body.Mode == "raw_json_arg" {
			arg, ok := argSet[action.Request.Body.Arg]
			if !ok {
				errs = append(errs, newValidationError(file, "request.body.arg", "must reference declared arg"))
			} else if arg.Type != "string" {
				errs = append(errs, newValidationError(file, "request.body.arg", "arg type must be string"))
			}
		}
	}

	for i, q := range action.Request.Query {
		if q.ArrayFormat != "" && q.ArrayFormat != "csv" && q.ArrayFormat != "repeat" && q.ArrayFormat != "brackets" {
			errs = append(errs, newValidationError(file, fmt.Sprintf("request.query[%d].array_format", i), "must be csv, repeat, or brackets"))
		}
		if s, ok := stringFromAny(q.Value); ok {
			kind, name, valid := parsePlaceholder(s)
			if valid && kind == "arg" {
				if arg, exists := argSet[name]; exists && arg.Type == "array" && q.ArrayFormat == "" {
					errs = append(errs, newValidationError(file, fmt.Sprintf("request.query[%d].array_format", i), "array arg requires array_format"))
				}
			}
		}
	}

	return errs
}
