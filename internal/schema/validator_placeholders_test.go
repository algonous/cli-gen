package schema

import "testing"

func TestValidatePlaceholdersValid(t *testing.T) {
	cli := validCLI()
	a := validAction()
	a.Request.Headers = []Header{{Name: "X-Debug", Value: "{env.GITHUB_TOKEN}"}}
	a.Request.Body = &BodyDef{
		Mode: "template",
		Template: map[string]any{
			"title": "{arg.owner}",
			"meta":  map[string]any{"repo": "{arg.repo}"},
		},
	}
	if errs := ValidatePlaceholders(cli, a); len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}
}

func TestValidatePlaceholdersViolations(t *testing.T) {
	cli := validCLI()
	a := validAction()
	a.Request.Path = "/repos/{env.GITHUB_TOKEN}/{arg.missing}"
	a.Request.Headers = []Header{{Name: "X-Test", Value: "{env.MISSING}"}}
	a.Request.Body = &BodyDef{
		Mode: "template",
		Template: map[string]any{
			"{arg.k}": "x",
			"payload": "{arg.payload}",
		},
	}
	a.Args = append(a.Args, ArgDef{Name: "payload", Type: "json", Help: "p"})

	errs := ValidatePlaceholders(cli, a)
	if len(errs) < 4 {
		t.Fatalf("expected many errors, got %+v", errs)
	}
}
