GITOPIA_ENV ?= testing

all: install

.PHONY: build

build:
		@go build -tags $(GITOPIA_ENV) -o build/ ./cmd/git-remote-gitopia
		@go build -tags $(GITOPIA_ENV) -o build/git-gitopia ./cmd/git-gitopia-keys

install: go.sum
		@echo "--> Installing git-remote-gitopia"
		@go install -tags $(GITOPIA_ENV) -mod=readonly ./cmd/git-remote-gitopia
		@go install -tags $(GITOPIA_ENV) -mod=readonly ./cmd/git-gitopia-keys
		mv $(GOBIN)/git-gitopia-keys $(GOBIN)/git-gitopia


go.sum: go.mod
		@echo "--> Ensure dependencies have not been modified"
		GO111MODULE=on go mod verify
