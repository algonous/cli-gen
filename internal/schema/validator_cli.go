package schema

import "fmt"

func ValidateCLIFile(cli *CliFile) []ValidationError {
	var errs []ValidationError
	file := "cli.yaml"

	if cli.SchemaVersion != "v1" {
		errs = append(errs, newValidationError(file, "schema_version", "must be v1"))
	}
	if !cliNamePattern.MatchString(cli.Cli.Name) {
		errs = append(errs, newValidationError(file, "cli.name", "must match ^[a-z][a-z0-9-]*$"))
	}
	if cli.Cli.Description == "" {
		errs = append(errs, newValidationError(file, "cli.description", "must be non-empty"))
	}
	if len(cli.Cli.Runtime.Env) == 0 {
		errs = append(errs, newValidationError(file, "cli.runtime.env", "must be non-empty"))
	}
	envSet := map[string]EnvEntry{}
	for i, env := range cli.Cli.Runtime.Env {
		field := fmt.Sprintf("cli.runtime.env[%d].name", i)
		if !envNamePattern.MatchString(env.Name) {
			errs = append(errs, newValidationError(file, field, "must match ^[A-Z][A-Z0-9_]*$"))
		}
		if _, ok := envSet[env.Name]; ok {
			errs = append(errs, newValidationError(file, field, fmt.Sprintf("duplicate env name %q", env.Name)))
		}
		envSet[env.Name] = env
	}
	if _, ok := envSet[cli.Cli.Runtime.BaseURL.FromEnv]; !ok {
		errs = append(errs, newValidationError(file, "cli.runtime.base_url.from_env", "must reference declared env"))
	}
	if cli.Cli.Runtime.Auth != nil {
		if cli.Cli.Runtime.Auth.Header == "" {
			errs = append(errs, newValidationError(file, "cli.runtime.auth.header", "must be non-empty"))
		}
		if cli.Cli.Runtime.Auth.Template == "" {
			errs = append(errs, newValidationError(file, "cli.runtime.auth.template", "must be non-empty"))
		}
		placeholderErrs := placeholderErrors(
			file,
			"cli.runtime.auth.template",
			cli.Cli.Runtime.Auth.Template,
			map[string]bool{"arg": true, "env": true},
			map[string]ArgDef{},
			envSet,
		)
		errs = append(errs, placeholderErrs...)
	}
	return errs
}
