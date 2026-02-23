package schema

import "testing"

func TestDetectUnusedArgs(t *testing.T) {
	a := validAction()
	if errs := DetectUnusedArgs(a); len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}

	a.Args = append(a.Args, ArgDef{Name: "extra", Type: "string", Help: "extra"})
	errs := DetectUnusedArgs(a)
	if len(errs) == 0 {
		t.Fatal("expected unused arg error")
	}
}

func TestDetectUnusedArgsRawJSONArg(t *testing.T) {
	a := &ActionFile{
		Name:    "custom",
		Request: RequestDef{Body: &BodyDef{Mode: "raw_json_arg", Arg: "payload-json"}},
		Args:    []ArgDef{{Name: "payload-json", Type: "string", Help: "payload"}},
	}
	if errs := DetectUnusedArgs(a); len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}
}
