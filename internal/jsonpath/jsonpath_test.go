package jsonpath

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := []struct {
		name string
		expr string
	}{
		{"root only", "$"},
		{"single field", "$.a"},
		{"nested fields", "$.a.b.c"},
		{"array index", "$[0]"},
		{"wildcard", "$[*]"},
		{"field then index", "$.data[0].id"},
		{"bracket quoted", "$['foo-bar']"},
		{"bracket quoted escaped quote", "$['a\\'b']"},
		{"bracket quoted escaped backslash", "$['a\\\\b']"},
		{"mixed dot and bracket", "$.headers['content-type']"},
		{"field then wildcard", "$.items[*]"},
	}

	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.expr); err != nil {
				t.Errorf("Validate(%q) returned error: %v", tc.expr, err)
			}
		})
	}

	invalid := []struct {
		name string
		expr string
		want string // substring of error message
	}{
		{"empty string", "", "empty expression"},
		{"no dollar", "abc", "must start with $"},
		{"recursive descent", "$..name", "recursive descent"},
		{"filter", "$.items[?(@.x>1)]", "filter"},
		{"function", "$.length()", "invalid JSONPath syntax"},
		{"negative index", "$[-1]", "negative array indices"},
		{"union", "$[1,2]", "union"},
		{"slice", "$[1:3]", "slice"},
		{"dot key invalid", "$.foo-bar", "invalid JSONPath syntax"},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.expr)
			if err == nil {
				t.Errorf("Validate(%q) expected error, got nil", tc.expr)
				return
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate(%q) error = %q, want substring %q", tc.expr, err.Error(), tc.want)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	tests := []struct {
		name string
		data any
		expr string
		want any
	}{
		{
			name: "root object",
			data: map[string]any{"a": 1.0},
			expr: "$",
			want: map[string]any{"a": 1.0},
		},
		{
			name: "single field",
			data: map[string]any{"a": 1.0},
			expr: "$.a",
			want: 1.0,
		},
		{
			name: "nested field",
			data: map[string]any{"a": map[string]any{"b": 2.0}},
			expr: "$.a.b",
			want: 2.0,
		},
		{
			name: "array index",
			data: map[string]any{"items": []any{10.0, 20.0}},
			expr: "$.items[0]",
			want: 10.0,
		},
		{
			name: "wildcard",
			data: map[string]any{"items": []any{1.0, 2.0, 3.0}},
			expr: "$.items[*]",
			want: []any{1.0, 2.0, 3.0},
		},
		{
			name: "composite path",
			data: map[string]any{
				"a": map[string]any{
					"b": []any{
						map[string]any{"c": "found"},
					},
				},
			},
			expr: "$.a.b[0].c",
			want: "found",
		},
		{
			name: "bracket quoted key",
			data: map[string]any{"foo-bar": "baz"},
			expr: "$['foo-bar']",
			want: "baz",
		},
		{
			name: "bracket quoted key escaped quote",
			data: map[string]any{"a'b": "quoted"},
			expr: "$['a\\'b']",
			want: "quoted",
		},
		{
			name: "bracket quoted key escaped backslash",
			data: map[string]any{"a\\b": "slash"},
			expr: "$['a\\\\b']",
			want: "slash",
		},
		{
			name: "root is array",
			data: []any{1.0, 2.0},
			expr: "$",
			want: []any{1.0, 2.0},
		},
		{
			name: "root is scalar",
			data: "hello",
			expr: "$",
			want: "hello",
		},
		{
			name: "wildcard nested field",
			data: map[string]any{
				"a": []any{
					map[string]any{"b": 1.0},
					map[string]any{"b": 2.0},
				},
			},
			expr: "$.a[*].b",
			want: []any{1.0, 2.0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Extract(tc.data, tc.expr)
			if err != nil {
				t.Fatalf("Extract returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Extract(%v, %q) = %v (%T), want %v (%T)",
					tc.data, tc.expr, got, got, tc.want, tc.want)
			}
		})
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name string
		data any
		expr string
	}{
		{
			name: "key not found",
			data: map[string]any{"a": 1.0},
			expr: "$.b",
		},
		{
			name: "array index out of bounds",
			data: map[string]any{"items": []any{1.0}},
			expr: "$.items[5]",
		},
		{
			name: "intermediate null",
			data: map[string]any{"a": nil},
			expr: "$.a.b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Extract(tc.data, tc.expr)
			if err == nil {
				t.Errorf("Extract(%v, %q) expected error, got nil", tc.data, tc.expr)
			}
		})
	}

	// Wildcard partial match: some elements have 'b', others don't
	t.Run("wildcard partial match", func(t *testing.T) {
		data := map[string]any{
			"a": []any{
				map[string]any{"b": 1.0},
				map[string]any{"c": 2.0},
				map[string]any{"b": 3.0},
			},
		}
		got, err := Extract(data, "$.a[*].b")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{1.0, 3.0}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	// Wildcard all missing: no elements have 'b'
	t.Run("wildcard all missing", func(t *testing.T) {
		data := map[string]any{
			"a": []any{
				map[string]any{"c": 1.0},
				map[string]any{"d": 2.0},
			},
		}
		got, err := Extract(data, "$.a[*].b")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v (%T), want %v (%T)", got, got, want, want)
		}
	})
}
