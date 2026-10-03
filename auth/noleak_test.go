package auth_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
)

type leakyClient struct{}

func (leakyClient) Fetch(context.Context, string) (auth.Token, error) {
	return auth.Token{}, errors.New("daemon rejected Bearer " + secret + secret)
}
func (leakyClient) Refresh(ctx context.Context, p string) (auth.Token, error) {
	return leakyClient{}.Fetch(ctx, p)
}

func TestNoTokenInErrors(t *testing.T) {
	src, _ := auth.NewDaemonTokenSource(leakyClient{}, "p")
	a := auth.NewAuthorizer(src)
	req, _ := http.NewRequest("GET", "http://x.invalid", nil)
	for _, err := range []error{a.Authorize(context.Background(), req), a.Refresh(context.Background())} {
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("leak or nil: %v", err)
		}
	}
}

// TestNoExportedFunctionReturnsTokenText is the CORE-AUTH-9 API check: no
// exported function or method of package auth returns a string or []byte,
// except the fixed list of formatting methods that are redacted or carry
// non-secret text.
func TestNoExportedFunctionReturnsTokenText(t *testing.T) {
	allowed := map[string]bool{
		"Error": true, "Hint": true, "String": true, "GoString": true,
		"MarshalText": true, "MarshalJSON": true, "Unwrap": false,
	}
	checked := 0
	{
		for _, f := range parseSources(t, 0) {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || !fd.Name.IsExported() || fd.Type.Results == nil {
					continue
				}
				if lower := strings.ToLower(fd.Name.Name); strings.Contains(lower, "reveal") || strings.Contains(lower, "print") || strings.Contains(lower, "raw") {
					t.Errorf("suspicious exported name %s", fd.Name.Name)
				}
				for _, r := range fd.Type.Results.List {
					checked++
					if isTextType(r.Type) && !allowed[fd.Name.Name] {
						t.Errorf("%s returns text/bytes", fd.Name.Name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("checked nothing")
	}
}

func isTextType(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "string"
	case *ast.ArrayType:
		id, ok := x.Elt.(*ast.Ident)
		return ok && (id.Name == "byte" || id.Name == "string")
	}
	return false
}

func TestNoImportOfHTTPXOrDaemonModule(t *testing.T) {
	for _, f := range parseSources(t, parser.ImportsOnly) {
		for _, im := range f.Imports {
			if strings.Contains(im.Path.Value, "httpx") || strings.Contains(im.Path.Value, "agent-okta-d") {
				t.Errorf("imports %s", im.Path.Value)
			}
		}
	}
}

// parseSources parses the package's non-test Go files.
func parseSources(t *testing.T, mode parser.Mode) []*ast.File {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), n, nil, mode)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return files
}
