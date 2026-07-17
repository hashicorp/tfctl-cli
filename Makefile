SHELL=/usr/bin/env bash
NAME=tfctl
BIN_PATH ?= dist/$(NAME)
ASSETS ?= assets

ifeq ($(GOARCH), arm64)
	GOARCH = arm64
else ifeq ($(GOARCH), s390x)
	GOARCH = s390x
else
	GOARCH = amd64
endif

default: help

.PHONY: linux
linux:
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) bin

.PHONY: docker
docker: linux
	docker build --build-arg BUILD_DIRECTORY="dist" -t hashicorp/$(NAME):latest .

.PHONY: bin
bin: $(BIN_PATH)

.PHONY: $(BIN_PATH)
$(BIN_PATH):
	CGO_ENABLED=0 go build -o $(BIN_PATH) -trimpath -buildvcs=false ./cmd/$(NAME)

.PHONY: clean
clean:
	rm -rf $(CURDIR)/$(dir $(BIN_PATH))

.PHONY: gen/screenshot
gen/screenshot: go/install # Create a screenshot of the tfctl CLI
	@go run github.com/homeport/termshot/cmd/termshot@v0.6.1 -c -f $(ASSETS)/tfctl.png -- tfctl

.PHONY: gen/logo
gen/logo: logotools
	@lolcat -S 26 -f <(figlet -d ./assets -f "Sub-Zero.flf" tfctl) > ./cmd/tfctl/logo.txt

.PHONY: go/build
go/build: bin

.PHONY: go/install
go/install:
	@go install ./cmd/tfctl

.PHONY: go/lint
go/lint:
	@golangci-lint run

.PHONY: go/test
go/test:
	@go test -v ./...

# Format code
.PHONY: go/fmt
go/fmt:
	@gofmt -s -w .

# Check formatting
.PHONY: fmt-check
fmt-check:
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || (echo "Code is not formatted. Run 'make go/fmt'" && exit 1)

# Install development tools
.PHONY: tools
tools:
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$$(cat .golangci-lint-version | head -n 1)
	@command -v changie >/dev/null 2>&1 || { \
		echo "Installing changie..."; \
		if command -v brew >/dev/null 2>&1; then brew install changie; \
		else echo "Could not auto-install changie: https://changie.dev/guide/installation/" && exit 1; \
		fi; \
	}

.PHONY: logotools
logotools:
	@command -v lolcat >/dev/null 2>&1 || { \
		echo "Installing lolcat..."; \
		if command -v brew >/dev/null 2>&1; then brew install lolcat; \
		else echo "Could not auto-install lolcat: https://github.com/busyloop/lolcat" && exit 1; \
		fi; \
	}
	@command -v figlet >/dev/null 2>&1 || { \
		echo "Install figlet https://www.figlet.org/" && exit 1; \
	}

.PHONY: check
check: fmt-check go/lint go/test

# Help (make usage)
.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Tools:"
	@echo " tools           Install development tools"
	@echo " gen/screenshot  Generate a screenshot of the CLI in $(ASSETS)/"
	@echo " gen/logo        Generate the ASCII art logo"
	@echo ""
	@echo "Build:"
	@echo " go/install      Install tfctl binary to GOPATH/bin"
	@echo " bin             Build a binary for tfctl ($(BIN_PATH))"
	@echo " clean           Clean build artifacts"
	@echo " docker          Build a docker image for tfctl"
	@echo ""
	@echo "Code:"
	@echo " check           Run all checks (formatting, linting, tests)"
	@echo " go/test         Run all tests"
	@echo " go/lint         Run golangci-lint"
	@echo " go/fmt          Format go code"
	@echo " fmt-check       Check go code formatting"
	@echo ""