all: install

build:
		@go build -o build/ ./cmd/git-remote-gitopia

install: go.sum
		@echo "--> Installing git-remote-gitopia"
		@go install -mod=readonly ./cmd/git-remote-gitopia

go.sum: go.mod
		@echo "--> Ensure dependencies have not been modified"
		GO111MODULE=on go mod verify
