package schema

import (
	"fmt"

	internaljsonpath "github.com/autonous/cli-gen/internal/jsonpath"
)

func ValidateResponseHints(action *ActionFile) []ValidationError {
	file := "actions/" + action.Name + ".yaml"
	if action.ResponseHints == nil {
		return nil
	}
	var errs []ValidationError
	for i, field := range action.ResponseHints.Fields {
		if err := internaljsonpath.Validate(field.Path); err != nil {
			errs = append(errs, newValidationError(file, fmt.Sprintf("response_hints.fields[%d].path", i), err.Error()))
		}
	}
	return errs
}
