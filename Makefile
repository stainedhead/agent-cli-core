# agent-cli-core is a library: there is no binary and no build target.
.PHONY: test lint fmt vet cover cross vuln fuzz tidy-check check

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

vet:
	go vet ./...

# Coverage for every package.
cover:
	go test -race -cover ./...

# Compile every package for the supported targets.
cross:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./... && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go vet ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./... && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go vet ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./... && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go vet ./...

# Vulnerability scan. Needs network; the version matches the CI workflow.
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

# Short fuzz run of the cross-package token-leak property.
fuzz:
	go test ./internal/integration -run xxx -fuzz FuzzNoTokenLeak -fuzztime 30s

# Fails when go.mod or go.sum are not tidy.
tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

# The full quality gate.
check:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "gofmt needed"; exit 1)
	$(MAKE) vet lint test cross
