SHELL=/usr/bin/env bash
NAME=tfctl
BIN_PATH ?= bin/$(NAME)

ifeq ($(GOARCH), arm64)
	GOARCH = arm64
else ifeq ($(GOARCH), s390x)
	GOARCH = s390x
else
	GOARCH = amd64
endif

default: $(BIN_PATH)

.PHONY: linux
linux:
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) bin

.PHONY: docker
docker: linux
	docker build --platform=linux/$(GOARCH) --build-arg BUILD_DIRECTORY="bin" -t hashicorp/$(NAME):latest .

.PHONY: bin
bin: $(BIN_PATH)

.PHONY: $(BIN_PATH)
$(BIN_PATH):
	CGO_ENABLED=0 go build -o $(BIN_PATH) -trimpath -buildvcs=false ./cmd/$(NAME)

.PHONY: clean
clean:
	rm -rf $(CURDIR)/$(dir $(BIN_PATH))

.PHONY: gen/screenshot
gen/screenshot: go/install ## Create a screenshot of the tfctl CLI
	@go run github.com/homeport/termshot/cmd/termshot@v0.6.1 -c -f assets/tfctl.png -- tfctl

.PHONY: go/build
go/build: bin

.PHONY: go/install
go/install:
	@go install ./cmd/tfctl

.PHONY: go/lint
go/lint:
	@golangci-lint run