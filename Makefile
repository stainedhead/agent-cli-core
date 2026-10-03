# agent-cli-core is a library: there is no binary and no build target.
.PHONY: test lint fmt vet

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

vet:
	go vet ./...
