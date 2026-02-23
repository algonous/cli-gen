package schema

import "testing"

func validCLI() *CliFile {
	return &CliFile{
		SchemaVersion: "v1",
		Cli: CliDef{
			Name:        "github",
			Description: "GitHub REST API CLI",
			Runtime: RuntimeDef{
				Env:     []EnvEntry{{Name: "GITHUB_BASE_URL", Required: true}, {Name: "GITHUB_TOKEN", Required: true, Secret: true}},
				BaseURL: BaseURLDef{FromEnv: "GITHUB_BASE_URL"},
				Auth:    &AuthDef{Header: "Authorization", Template: "Bearer {env.GITHUB_TOKEN}"},
			},
		},
	}
}

func TestValidateCLIFileValid(t *testing.T) {
	if errs := ValidateCLIFile(validCLI()); len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
}

func TestValidateCLIFileViolations(t *testing.T) {
	cli := validCLI()
	cli.SchemaVersion = "v2"
	cli.Cli.Name = "GitHub"
	cli.Cli.Description = ""
	cli.Cli.Runtime.Env = []EnvEntry{{Name: "bad-name"}, {Name: "bad-name"}}
	cli.Cli.Runtime.BaseURL.FromEnv = "MISSING"
	cli.Cli.Runtime.Auth = &AuthDef{Header: "", Template: "Bearer {env.MISSING}"}

	errs := ValidateCLIFile(cli)
	if len(errs) < 7 {
		t.Fatalf("expected multiple errors, got %+v", errs)
	}
}
