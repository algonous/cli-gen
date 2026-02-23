package jsonpath

import (
	"errors"
	"fmt"

	"github.com/ohler55/ojg/jp"
)

// Validate checks whether expr is within the allowed JSONPath subset.
// Returns nil if valid, or a descriptive error.
func Validate(expr string) error {
	if expr == "" {
		return errors.New("empty expression")
	}

	parsed, err := jp.ParseString(expr)
	if err != nil {
		return fmt.Errorf("invalid JSONPath syntax: %w", err)
	}

	if len(parsed) == 0 {
		return errors.New("empty expression")
	}

	if _, ok := parsed[0].(jp.Root); !ok {
		return errors.New("expression must start with $")
	}

	for _, frag := range parsed {
		switch f := frag.(type) {
		case jp.Root:
			// allowed
		case jp.Child:
			// allowed
		case jp.Nth:
			if int(f) < 0 {
				return errors.New("negative array indices are not supported")
			}
		case jp.Wildcard:
			// allowed
		case jp.Descent:
			return errors.New("recursive descent (..) is not supported")
		case jp.Slice:
			return errors.New("slice expressions are not supported")
		case jp.Union:
			return errors.New("union expressions are not supported")
		case *jp.Filter:
			return errors.New("filter expressions are not supported")
		default:
			return fmt.Errorf("unsupported expression: %T", frag)
		}
	}

	return nil
}

// Extract evaluates expr against data and returns the result.
// The caller must ensure expr has been validated with Validate first.
func Extract(data any, expr string) (any, error) {
	parsed, err := jp.ParseString(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid JSONPath syntax: %w", err)
	}

	results := parsed.Get(data)

	if containsWildcard(parsed) {
		if results == nil {
			return []any{}, nil
		}
		return results, nil
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("path not found: %s", expr)
	}
	return results[0], nil
}

func containsWildcard(expr jp.Expr) bool {
	for _, frag := range expr {
		if _, ok := frag.(jp.Wildcard); ok {
			return true
		}
	}
	return false
}
