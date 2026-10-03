package output_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stainedhead/agent-cli-core/output"
)

type catErr struct{ c output.Category }

func (e catErr) Error() string             { return "boom" }
func (e catErr) Category() output.Category { return e.c }

func TestExitCodeValues(t *testing.T) {
	want := map[output.ExitCode]int{
		output.ExitOK: 0, output.ExitGeneral: 1, output.ExitUsage: 2, output.ExitAuth: 3,
		output.ExitForbidden: 4, output.ExitNotFound: 5, output.ExitPolicyDenied: 6,
		output.ExitConflict: 7, output.ExitRateLimited: 8, output.ExitValidation: 9,
	}
	for c, n := range want {
		if int(c) != n {
			t.Errorf("%v != %d", c, n)
		}
	}
}

func TestEveryCategoryMapsToExactlyOneCode(t *testing.T) {
	seen := map[output.ExitCode]output.Category{}
	for i, c := range output.Categories() {
		code := output.ExitFor(c)
		if int(code) != i {
			t.Errorf("%s -> %d, want %d", c, code, i)
		}
		if prev, dup := seen[code]; dup {
			t.Errorf("%s and %s share code %d", prev, c, code)
		}
		seen[code] = c
	}
	if len(seen) != 10 {
		t.Fatalf("want 10 categories, got %d", len(seen))
	}
}

func TestUnknownCategoryIsGeneral(t *testing.T) {
	if got := output.ExitFor("nonsense"); got != output.ExitGeneral {
		t.Fatalf("got %d", got)
	}
	if got := output.ExitFor(""); got != output.ExitGeneral {
		t.Fatalf("got %d", got)
	}
}

func TestCategoryOf(t *testing.T) {
	wrapped := fmt.Errorf("ctx: %w", catErr{output.CategoryAuth})
	cases := []struct {
		name string
		err  error
		cat  output.Category
		exit output.ExitCode
	}{
		{"nil", nil, output.CategoryOK, 0},
		{"plain", errors.New("x"), output.CategoryGeneral, 1},
		{"direct", catErr{output.CategoryForbidden}, output.CategoryForbidden, 4},
		{"wrapped", wrapped, output.CategoryAuth, 3},
		{"joined", errors.Join(errors.New("a"), catErr{output.CategoryRateLimited}), output.CategoryRateLimited, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := output.CategoryOf(c.err); got != c.cat {
				t.Errorf("category %q want %q", got, c.cat)
			}
			if got := output.ExitOf(c.err); got != c.exit {
				t.Errorf("exit %d want %d", got, c.exit)
			}
		})
	}
}

func ExampleExitOf() {
	var err error = catErr{output.CategoryPolicyDenied}
	fmt.Println(output.ExitOf(err))
	// Output: 6
}
