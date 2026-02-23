package schema

import "testing"

func validAction() *ActionFile {
	return &ActionFile{
		Name:        "list-repo-issues",
		Impl:        "generated",
		Description: "List issues",
		Request: RequestDef{
			Method: "GET",
			Path:   "/repos/{arg.owner}/{arg.repo}/issues",
			Query:  []QueryParam{{Name: "labels", Value: "{arg.labels}", ArrayFormat: "repeat"}},
		},
		Args: []ArgDef{
			{Name: "owner", Type: "string", Required: true, Help: "owner"},
			{Name: "repo", Type: "string", Required: true, Help: "repo"},
			{Name: "labels", Type: "array", Required: false, Help: "labels"},
		},
		Response: ResponseDef{SuccessStatus: []int{200}},
	}
}

func TestValidateActionFileValid(t *testing.T) {
	if errs := ValidateActionFile(validAction()); len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
}

func TestValidateActionFileViolations(t *testing.T) {
	a := validAction()
	a.Name = "Bad"
	a.Impl = ""
	a.Description = ""
	a.Request.Method = "TRACE"
	a.Request.Path = ""
	a.Args[0].Name = "bad_name"
	a.Args[0].Type = "bad"
	a.Args[0].Help = ""
	a.Response.SuccessStatus = nil
	a.Request.Body = &BodyDef{Mode: "raw_json_arg", Arg: "owner"}
	a.Request.Query[0].ArrayFormat = ""

	errs := ValidateActionFile(a)
	if len(errs) < 9 {
		t.Fatalf("expected many errors, got %+v", errs)
	}
}
