package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"testing"

	"sigs.k8s.io/yaml"
)

// Compare actual event emissions with the public contract, so adding a new
// lifecycle or observation event cannot silently break strict API clients.
func TestOpenAPIIncludesEmittedEvents(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err = yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, kind := range spec.Components.Schemas["Event"].Properties["kind"].Enum {
		allowed[kind] = true
	}
	source, err := parser.ParseFile(token.NewFileSet(), "../repository/store.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || name.Name != "addEvent" || len(call.Args) != 5 {
			return true
		}
		literal, ok := call.Args[3].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		kind, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed[kind] {
			t.Errorf("emitted event %s is missing from OpenAPI", kind)
		}
		return true
	})
	for _, kind := range []string{"SUCCEEDED", "FAILED", "TIMED_OUT"} {
		if !allowed[kind] {
			t.Errorf("terminal event %s is missing from OpenAPI", kind)
		}
	}
}
