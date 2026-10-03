package policy

// The YAML dependency is declared in go.mod by the foundation workstream so
// that policy work never has to change go.mod. This blank import keeps
// `go mod tidy` stable until the strict parser uses the package for real; the
// policy workstream may replace this file.
import _ "github.com/goccy/go-yaml"
