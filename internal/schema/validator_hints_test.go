package schema

import "testing"

func TestValidateResponseHints(t *testing.T) {
	a := &ActionFile{Name: "a", ResponseHints: &ResponseHints{Fields: []HintField{{Path: "$"}, {Path: "$.number"}, {Path: "$[0].number"}}}}
	if errs := ValidateResponseHints(a); len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}

	a.ResponseHints.Fields = []HintField{{Path: "$..name"}, {Path: ""}}
	errs := ValidateResponseHints(a)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %+v", errs)
	}
}
