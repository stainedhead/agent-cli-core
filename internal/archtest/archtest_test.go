package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stainedhead/agent-cli-core/internal/archtest"
)

const mod = "example.com/lib"

func src(imports ...string) *fstest.MapFile {
	var b strings.Builder
	b.WriteString("package p\n")
	for _, i := range imports {
		b.WriteString("import _ \"" + i + "\"\n")
	}
	return &fstest.MapFile{Data: []byte(b.String())}
}

func rules() archtest.Rules {
	return archtest.Rules{
		Module: mod,
		Allowed: map[string][]string{
			"output": {"internal/redact"},
			"httpx":  {"output", "internal/redact"},
			"auth":   {"output", "internal/redact"},
		},
		External: map[string][]string{"policy": {"gopkg.in/yaml.v3"}},
	}
}

func kinds(vs []archtest.Violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Kind)
	}
	return out
}

func TestCleanTreePasses(t *testing.T) {
	fsys := fstest.MapFS{
		"output/a.go":          src(mod + "/internal/redact"),
		"internal/redact/a.go": src("strings"),
		"httpx/a.go":           src(mod+"/output", "net/http"),
		"policy/a.go":          src("gopkg.in/yaml.v3"),
		"auth/a_test.go":       src(mod + "/auth/authtest"), // tests are not part of the graph
		"auth/authtest/a.go":   src(mod + "/auth"),
		"auth/a.go":            src(mod + "/output"),
	}
	if vs := archtest.Check(fsys, rules()); len(vs) != 0 {
		t.Fatalf("unexpected: %v", vs)
	}
}

func TestForbiddenEdge(t *testing.T) {
	fsys := fstest.MapFS{
		"auth/a.go":   src(mod + "/httpx"),
		"httpx/a.go":  src("strings"),
		"output/a.go": src(),
	}
	vs := archtest.Check(fsys, rules())
	if len(vs) != 1 || vs[0].Kind != archtest.KindForbiddenImport || !strings.Contains(vs[0].Detail, "auth -> httpx") {
		t.Fatalf("got %v", vs)
	}
}

func TestOwnSubtreeAllowed(t *testing.T) {
	fsys := fstest.MapFS{
		"auth/a.go":            src(mod + "/auth/internal/x"),
		"auth/internal/x/a.go": src(),
	}
	if vs := archtest.Check(fsys, rules()); len(vs) != 0 {
		t.Fatalf("got %v", vs)
	}
}

func TestCycle(t *testing.T) {
	fsys := fstest.MapFS{
		"a/a.go": src(mod + "/b"),
		"b/a.go": src(mod + "/c"),
		"c/a.go": src(mod + "/a"),
	}
	vs := archtest.Check(fsys, archtest.Rules{Module: mod})
	found := false
	for _, v := range vs {
		if v.Kind == archtest.KindCycle && strings.Contains(v.Detail, "a -> b -> c -> a") {
			found = true
		}
	}
	if !found {
		t.Fatalf("cycle not reported: %v", vs)
	}
}

func TestExternalImportNotAllowed(t *testing.T) {
	fsys := fstest.MapFS{
		"output/a.go": src("github.com/some/lib"),
		"policy/a.go": src("github.com/other/yaml"),
	}
	vs := archtest.Check(fsys, rules())
	if got := kinds(vs); len(got) != 2 || got[0] != archtest.KindExternalImport {
		t.Fatalf("got %v", vs)
	}
}

func TestVendorNameInImportPath(t *testing.T) {
	fsys := fstest.MapFS{"output/a.go": src("github.com/acme/ServiceNow-sdk")}
	vs := archtest.Check(fsys, rules())
	has := false
	for _, v := range vs {
		if v.Kind == archtest.KindVendorName {
			has = true
		}
	}
	if !has {
		t.Fatalf("got %v", vs)
	}
}

func TestVendorNameInIdentifiers(t *testing.T) {
	cases := map[string]string{
		"func":   "package p\nfunc FetchOutlookMail() {}\n",
		"type":   "package p\ntype SnowClient struct{}\n",
		"field":  "package p\ntype T struct{ OktaID string }\n",
		"var":    "package p\nvar teamsChannel = 1\n",
		"method": "package p\ntype T int\nfunc (T) GetServiceNowRow() {}\n",
		"const":  "package p\nconst msgraphURL = \"x\"\n",
		"pkg":    "package oktahelper\n",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{"output/a.go": {Data: []byte(code)}}
			vs := archtest.Check(fsys, rules())
			if len(vs) == 0 || vs[0].Kind != archtest.KindVendorName {
				t.Fatalf("got %v", vs)
			}
		})
	}
}

func TestVendorNameInPath(t *testing.T) {
	fsys := fstest.MapFS{"internal/snowflake/a.go": src()}
	vs := archtest.Check(fsys, rules())
	if len(vs) == 0 || vs[0].Kind != archtest.KindVendorName {
		t.Fatalf("got %v", vs)
	}
}

func TestVendorNamesInStringsAndCommentsAreAllowed(t *testing.T) {
	code := "package p\n// Callers such as snow, outlook and teams use this.\nconst doc = \"outlook okta\"\n"
	fsys := fstest.MapFS{"output/a.go": {Data: []byte(code)}}
	if vs := archtest.Check(fsys, rules()); len(vs) != 0 {
		t.Fatalf("got %v", vs)
	}
}

func TestVendorNamesAlsoCheckedInTestFiles(t *testing.T) {
	fsys := fstest.MapFS{"output/a_test.go": {Data: []byte("package p\nfunc TestSnow() {}\n")}}
	if vs := archtest.Check(fsys, rules()); len(vs) == 0 {
		t.Fatal("expected a violation")
	}
}

func TestUnparsableFileIsReported(t *testing.T) {
	fsys := fstest.MapFS{"output/a.go": {Data: []byte("not go")}}
	vs := archtest.Check(fsys, rules())
	if len(vs) != 1 || vs[0].Kind != archtest.KindParse {
		t.Fatalf("got %v", vs)
	}
}

func TestViolationString(t *testing.T) {
	v := archtest.Violation{Kind: "k", Path: "p", Detail: "d"}
	if got := v.String(); got != "k: p: d" {
		t.Fatal(got)
	}
	if got := (archtest.Violation{Kind: "k", Detail: "d"}).String(); got != "k: d" {
		t.Fatal(got)
	}
}

// TestRepositoryObeysArchitecture is the real dependency-rule test (FR-025):
// it checks every package of this module.
func TestRepositoryObeysArchitecture(t *testing.T) {
	root := repoRoot(t)
	for _, v := range archtest.Check(os.DirFS(root), archtest.DefaultRules()) {
		t.Error(v)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
