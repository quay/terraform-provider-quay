HOSTNAME    = registry.terraform.io
NAMESPACE   = quay
NAME        = quay
BINARY      = terraform-provider-$(NAME)
VERSION     ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0-dev")

OS_ARCH     = $(shell go env GOOS)_$(shell go env GOARCH)

default: build

.PHONY: generate
generate:
	go generate ./...

.PHONY: generate-check
generate-check: generate
	@git diff --exit-code docs/ examples/ templates/ || \
		(echo "\nGenerated docs are out of sync. Run 'make generate' and commit the result." && exit 1)

.PHONY: build
build:
	go build -o $(BINARY)

.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(VERSION)/$(OS_ARCH)
	mv $(BINARY) ~/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(VERSION)/$(OS_ARCH)

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	gofmt -s -w .

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test -race ./... -v -count=1 -timeout 120s

.PHONY: testacc
testacc:
	TF_ACC=1 go test -race ./internal/provider/... -v -count=1 -timeout 600s $(TESTARGS)

.PHONY: clean
clean:
	rm -f $(BINARY)
