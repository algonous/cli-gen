package schema

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	cliNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	envNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	argNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	placeholderRe  = regexp.MustCompile(`\{[^}]+\}`)
)

func newValidationError(file, field, msg string) ValidationError {
	return ValidationError{File: file, Field: field, Message: msg}
}

func stringFromAny(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func extractPlaceholders(s string) []string {
	return placeholderRe.FindAllString(s, -1)
}

func parsePlaceholder(ph string) (kind string, name string, ok bool) {
	if len(ph) < 2 || ph[0] != '{' || ph[len(ph)-1] != '}' {
		return "", "", false
	}
	inner := ph[1 : len(ph)-1]
	parts := strings.Split(inner, ".")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[0] != "arg" && parts[0] != "env" {
		return "", "", false
	}
	if parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func flattenTemplateStrings(value any, includeKeys bool) (values []string, keys []string) {
	walkTemplate(value, includeKeys, &values, &keys)
	return values, keys
}

func walkTemplate(value any, includeKeys bool, values *[]string, keys *[]string) {
	switch vv := value.(type) {
	case map[string]any:
		for k, item := range vv {
			if includeKeys {
				*keys = append(*keys, k)
			}
			walkTemplate(item, includeKeys, values, keys)
		}
	case []any:
		for _, item := range vv {
			walkTemplate(item, includeKeys, values, keys)
		}
	case string:
		*values = append(*values, vv)
	}
}

func placeholderErrors(file, fieldPrefix, src string, allowedKinds map[string]bool, argSet map[string]ArgDef, envSet map[string]EnvEntry) []ValidationError {
	var errs []ValidationError
	for _, ph := range extractPlaceholders(src) {
		kind, name, ok := parsePlaceholder(ph)
		if !ok {
			errs = append(errs, newValidationError(file, fieldPrefix, fmt.Sprintf("invalid placeholder syntax: %s", ph)))
			continue
		}
		if !allowedKinds[kind] {
			errs = append(errs, newValidationError(file, fieldPrefix, fmt.Sprintf("placeholder kind %q is not allowed here", kind)))
			continue
		}
		if kind == "arg" {
			if _, ok := argSet[name]; !ok {
				errs = append(errs, newValidationError(file, fieldPrefix, fmt.Sprintf("arg %q is not declared", name)))
			}
		}
		if kind == "env" {
			if _, ok := envSet[name]; !ok {
				errs = append(errs, newValidationError(file, fieldPrefix, fmt.Sprintf("env %q is not declared", name)))
			}
		}
	}
	return errs
}
