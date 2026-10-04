// Package archtest enforces the library's architecture rules by reading its
// source: the allowed import graph, no import cycles, no third-party
// dependencies beyond the declared ones, and no vendor names in import paths,
// identifiers or file paths (the library knows nothing of any vendor).
//
// It is internal test support: the real check is the test
// TestRepositoryObeysArchitecture, which integration tests may call again.
package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Violation kinds.
const (
	KindForbiddenImport = "forbidden-import"
	KindExternalImport  = "external-import"
	KindCycle           = "cycle"
	KindVendorName      = "vendor-name"
	KindParse           = "parse"
)

// Violation is one broken rule.
type Violation struct {
	Kind   string
	Path   string // file or directory, when known
	Detail string
}

func (v Violation) String() string {
	if v.Path == "" {
		return v.Kind + ": " + v.Detail
	}
	return v.Kind + ": " + v.Path + ": " + v.Detail
}

// Rules describes the architecture.
type Rules struct {
	// Module is the module path, for example
	// github.com/stainedhead/agent-cli-core.
	Module string
	// Allowed maps a package directory (relative to the module root) to the
	// intra-module packages it may import. A package may always import its
	// own subtree. A package that is not listed is not restricted here (it is
	// still checked for cycles and vendor names).
	Allowed map[string][]string
	// External maps a package directory to the non-standard-library import
	// path prefixes it may use. A listed or unlisted package importing
	// anything outside the standard library, the module and this list is a
	// violation.
	External map[string][]string
	// VendorNames are case-insensitive substrings that must not appear in an
	// import path, an identifier or a file path.
	VendorNames []string
	// VendorExempt lists package directories (relative to the module root)
	// whose subtree is exempt from the vendor-name rule: the directory and
	// file names, identifiers and import paths there may contain a vendor
	// name. It is for edge adapters that must name the daemon they wrap. The
	// import allow-lists (Allowed, External) still apply to them.
	VendorExempt []string
}

// DefaultRules returns the rules of this repository (see the architecture
// document: leaf, contract, behavior and tooling layers).
func DefaultRules() Rules {
	return Rules{
		Module: "github.com/stainedhead/agent-cli-core",
		Allowed: map[string][]string{
			"clock":           {},
			"internal/clock":  {"clock"},
			"internal/redact": {},
			"output":          {"internal/redact"},
			"httpx":           {"output", "internal/redact", "internal/clock"},
			"auth":            {"output", "internal/redact", "internal/clock"},
			"auth/authtest":   {"auth", "output", "internal/clock"},
			"auth/oktad":      {"auth", "output"},
			"policy":          {"output"},
			"audit":           {"internal/redact", "internal/clock", "clock"},
			"selftest":        {"output"},
			"docgen":          {"output"},
		},
		External: map[string][]string{
			"policy":     {"github.com/goccy/go-yaml"},
			"auth/oktad": {"github.com/stainedhead/agent-okta-d/pkg/client"},
		},
		VendorNames: []string{
			"snow", "servicenow", "outlook", "teams", "okta", "msgraph",
			"microsoft", "office365", "sharepoint", "azuread",
		},
		// auth/oktad is the one adapter over the credential daemon's client;
		// it has to name the daemon.
		VendorExempt: []string{"auth/oktad"},
	}
}

// Check walks fsys (rooted at the module root) and returns every violation,
// sorted. Test files (_test.go) are ignored for the import graph and
// dependency rules, because external test packages may legitimately import
// their package's helpers; they are still checked for vendor names.
func Check(fsys fs.FS, r Rules) []Violation {
	if len(r.VendorNames) == 0 {
		r.VendorNames = DefaultRules().VendorNames
	}
	var vs []Violation
	graph := map[string]map[string]bool{}

	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != "." && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return fs.SkipDir
			}
			if v, bad := vendorIn(p, r.VendorNames); bad && p != "." && !r.vendorExempt(p) {
				vs = append(vs, Violation{KindVendorName, p, "directory name contains " + strconv.Quote(v)})
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		if v, bad := vendorIn(name, r.VendorNames); bad && !r.vendorExempt(p) {
			vs = append(vs, Violation{KindVendorName, p, "file name contains " + strconv.Quote(v)})
		}
		vs = append(vs, checkFile(fsys, p, r, graph)...)
		return nil
	})

	vs = append(vs, findCycles(graph)...)
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].Kind != vs[j].Kind {
			return vs[i].Kind < vs[j].Kind
		}
		if vs[i].Path != vs[j].Path {
			return vs[i].Path < vs[j].Path
		}
		return vs[i].Detail < vs[j].Detail
	})
	return vs
}

func checkFile(fsys fs.FS, p string, r Rules, graph map[string]map[string]bool) []Violation {
	var vs []Violation
	src, err := fs.ReadFile(fsys, p)
	if err != nil {
		return []Violation{{KindParse, p, err.Error()}}
	}
	f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.SkipObjectResolution)
	if err != nil {
		return []Violation{{KindParse, p, err.Error()}}
	}

	// Vendor names in identifiers (not in comments or string literals).
	exempt := r.vendorExempt(p)
	seen := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name == "_" || seen[id.Name] || exempt {
			return true
		}
		seen[id.Name] = true
		if v, bad := vendorIn(id.Name, r.VendorNames); bad {
			vs = append(vs, Violation{KindVendorName, p, "identifier " + id.Name + " contains " + strconv.Quote(v)})
		}
		return true
	})

	isTest := strings.HasSuffix(p, "_test.go")
	pkg := path.Dir(p)
	for _, imp := range f.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		if v, bad := vendorIn(ip, r.VendorNames); bad && !exempt {
			vs = append(vs, Violation{KindVendorName, p, "import " + ip + " contains " + strconv.Quote(v)})
		}
		if isTest {
			continue
		}
		switch {
		case ip == r.Module || strings.HasPrefix(ip, r.Module+"/"):
			target := strings.TrimPrefix(strings.TrimPrefix(ip, r.Module), "/")
			if graph[pkg] == nil {
				graph[pkg] = map[string]bool{}
			}
			graph[pkg][target] = true
			if allowed, restricted := r.Allowed[pkg]; restricted && !within(target, pkg) && !contains(allowed, target) {
				vs = append(vs, Violation{KindForbiddenImport, p, fmt.Sprintf("%s -> %s is not an allowed dependency", pkg, target)})
			}
		case isStdlib(ip):
		default:
			if !hasPrefixIn(ip, r.External[pkg]) {
				vs = append(vs, Violation{KindExternalImport, p, "import " + ip + " is not allowed in " + pkg})
			}
		}
	}
	return vs
}

// vendorExempt reports whether p (a directory or file path) is inside a
// VendorExempt subtree.
func (r Rules) vendorExempt(p string) bool {
	for _, d := range r.VendorExempt {
		if within(p, d) {
			return true
		}
	}
	return false
}

func vendorIn(s string, names []string) (string, bool) {
	l := strings.ToLower(s)
	for _, n := range names {
		if strings.Contains(l, n) {
			return n, true
		}
	}
	return "", false
}

func isStdlib(ip string) bool {
	first, _, _ := strings.Cut(ip, "/")
	return !strings.Contains(first, ".")
}

func within(target, pkg string) bool {
	return target == pkg || strings.HasPrefix(target, pkg+"/")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func hasPrefixIn(ip string, prefixes []string) bool {
	for _, p := range prefixes {
		if ip == p || strings.HasPrefix(ip, p+"/") || strings.HasPrefix(ip, p+".") {
			return true
		}
	}
	return false
}

// findCycles reports each import cycle once, in a stable form.
func findCycles(graph map[string]map[string]bool) []Violation {
	nodes := make([]string, 0, len(graph))
	for n := range graph {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var out []Violation
	reported := map[string]bool{}

	var visit func(n string)
	visit = func(n string) {
		color[n] = grey
		stack = append(stack, n)
		next := make([]string, 0, len(graph[n]))
		for m := range graph[n] {
			next = append(next, m)
		}
		sort.Strings(next)
		for _, m := range next {
			switch color[m] {
			case white:
				visit(m)
			case grey:
				i := len(stack) - 1
				for stack[i] != m {
					i--
				}
				cyc := append(append([]string(nil), stack[i:]...), m)
				key := strings.Join(canonical(cyc[:len(cyc)-1]), ",")
				if !reported[key] {
					reported[key] = true
					out = append(out, Violation{Kind: KindCycle, Detail: strings.Join(cyc, " -> ")})
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
	}
	for _, n := range nodes {
		if color[n] == white {
			visit(n)
		}
	}
	return out
}

// canonical rotates a cycle so equal cycles compare equal.
func canonical(c []string) []string {
	min := 0
	for i := range c {
		if c[i] < c[min] {
			min = i
		}
	}
	return append(append([]string(nil), c[min:]...), c[:min]...)
}
