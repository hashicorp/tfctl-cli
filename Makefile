SHELL=/usr/bin/env bash
NAME=tfcloud
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
	CGO_ENABLED=0 go build -o $(BIN_PATH) -trimpath -buildvcs=false ./cmd

.PHONY: clean
clean:
	rm -rf $(CURDIR)/$(dir $(BIN_PATH))

