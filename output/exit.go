package output

import "errors"

// ExitCode is a process exit code. The values are part of the public
// contract; changing one is a breaking change.
type ExitCode int

// The exit codes shared by every CLI built on this library.
const (
	// ExitOK means success.
	ExitOK ExitCode = 0
	// ExitGeneral is a general error.
	ExitGeneral ExitCode = 1
	// ExitUsage means the command line was malformed.
	ExitUsage ExitCode = 2
	// ExitAuth covers reauth_required, a second 401 and an unreachable
	// credential daemon.
	ExitAuth ExitCode = 3
	// ExitForbidden means the server refused the action (403 or ACL).
	ExitForbidden ExitCode = 4
	// ExitNotFound means the target does not exist.
	ExitNotFound ExitCode = 5
	// ExitPolicyDenied means client-side policy refused the action.
	ExitPolicyDenied ExitCode = 6
	// ExitConflict means a conflict or failed precondition.
	ExitConflict ExitCode = 7
	// ExitRateLimited means rate limiting or a transient failure persisted
	// after bounded retries.
	ExitRateLimited ExitCode = 8
	// ExitValidation means input failed validation, for example a mandatory
	// field is missing.
	ExitValidation ExitCode = 9
)

// Category is the stable machine-readable class of a result. It is the value
// of error.code in the envelope.
type Category string

// The categories, one per exit code.
const (
	CategoryOK           Category = "ok"
	CategoryGeneral      Category = "general"
	CategoryUsage        Category = "usage"
	CategoryAuth         Category = "auth"
	CategoryForbidden    Category = "forbidden"
	CategoryNotFound     Category = "not_found"
	CategoryPolicyDenied Category = "policy_denied"
	CategoryConflict     Category = "conflict"
	CategoryRateLimited  Category = "rate_limited"
	CategoryValidation   Category = "validation"
)

var categoryExit = map[Category]ExitCode{
	CategoryOK:           ExitOK,
	CategoryGeneral:      ExitGeneral,
	CategoryUsage:        ExitUsage,
	CategoryAuth:         ExitAuth,
	CategoryForbidden:    ExitForbidden,
	CategoryNotFound:     ExitNotFound,
	CategoryPolicyDenied: ExitPolicyDenied,
	CategoryConflict:     ExitConflict,
	CategoryRateLimited:  ExitRateLimited,
	CategoryValidation:   ExitValidation,
}

// Categories returns every category in exit-code order (0 to 9).
func Categories() []Category {
	return []Category{
		CategoryOK, CategoryGeneral, CategoryUsage, CategoryAuth, CategoryForbidden,
		CategoryNotFound, CategoryPolicyDenied, CategoryConflict, CategoryRateLimited,
		CategoryValidation,
	}
}

// ExitFor maps a category to its exit code. An unknown category maps to
// ExitGeneral.
func ExitFor(c Category) ExitCode {
	if code, ok := categoryExit[c]; ok {
		return code
	}
	return ExitGeneral
}

// CategoryError is implemented by errors that know their category. Packages
// make their errors map to an exit code by implementing it, without
// importing each other.
type CategoryError interface {
	error
	Category() Category
}

// Hinter is optionally implemented by errors that carry an actionable hint
// for the caller. FromError uses it for error.hint.
type Hinter interface {
	Hint() string
}

// CategoryOf returns the category of err: CategoryOK for nil, the category of
// the first CategoryError in the chain, otherwise CategoryGeneral.
func CategoryOf(err error) Category {
	if err == nil {
		return CategoryOK
	}
	var ce CategoryError
	if errors.As(err, &ce) {
		return ce.Category()
	}
	return CategoryGeneral
}

// ExitOf returns the exit code for err (see CategoryOf and ExitFor).
func ExitOf(err error) ExitCode { return ExitFor(CategoryOf(err)) }
