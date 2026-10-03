# agent-cli-core is a library: there is no binary and no build target.
.PHONY: test lint fmt vet cover cross tidy-check check

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

vet:
	go vet ./...

# Coverage for the packages that must stay at or above 90 percent.
cover:
	go test -race -cover ./output/... ./internal/...

# Compile every package for the supported targets.
cross:
	GOOS=darwin GOARCH=arm64 go build ./...
	GOOS=linux GOARCH=amd64 go build ./...
	GOOS=linux GOARCH=arm64 go build ./...

# Fails when go.mod or go.sum are not tidy.
tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

# The full quality gate.
check:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "gofmt needed"; exit 1)
	$(MAKE) vet lint test cross
