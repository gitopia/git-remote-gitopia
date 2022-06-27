GITOPIA_ENV ?= prod
LEDGER_ENABLED ?= true

build_tags = netgo
ifeq ($(LEDGER_ENABLED),true)
  ifeq ($(OS),Windows_NT)
    GCCEXE = $(shell where gcc.exe 2> NUL)
    ifeq ($(GCCEXE),)
      $(error gcc.exe not installed for ledger support, please install or set LEDGER_ENABLED=false)
    else
      build_tags += ledger
    endif
  else
    UNAME_S = $(shell uname -s)
    ifeq ($(UNAME_S),OpenBSD)
      $(warning OpenBSD detected, disabling ledger support (https://github.com/cosmos/cosmos-sdk/issues/1988))
    else
      GCC = $(shell command -v gcc 2> /dev/null)
      ifeq ($(GCC),)
        $(error gcc not installed for ledger support, please install or set LEDGER_ENABLED=false)
      else
        build_tags += ledger
      endif
    endif
  endif
endif

build_tags += $(BUILD_TAGS)
build_tags := $(strip $(build_tags))
BUILD_FLAGS := -tags "$(build_tags) $(GITOPIA_ENV)"

appname := git-remote-gitopia
version := 0.4.2

<<<<<<< HEAD
build = GOOS=$(1) GOARCH=$(2) go build $(BUILD_FLAGS) -o build/$(appname)$(3) ./cmd/git-remote-gitopia && \
    GOOS=$(1) GOARCH=$(2) go build $(BUILD_FLAGS) -o build/git-gitopia$(3) ./cmd/git-gitopia-keys
tar = cd build && tar -cvzf $(appname)_$(version)_$(1)_$(2).tar.gz $(appname)$(3) git-gitopia$(3) \
    && rm $(appname)$(3) && rm git-gitopia$(3)
zip = cd build && zip $(appname)_$(version)_$(1)_$(2).zip $(appname)$(3) && rm $(appname)$(3) && \
    zip $(appname)_$(version)_$(1)_$(2).zip git-gitopia$(3) && rm git-gitopia$(3) 
=======
build = GOOS=$(1) GOARCH=$(2) go build $(BUILD_FLAGS) -o build/$(appname)$(3) ./cmd/git-remote-gitopia
tar = cd build && tar -cvzf $(appname)_$(version)_$(1)_$(2).tar.gz $(appname)$(3) && rm $(appname)$(3)
zip = cd build && zip $(appname)_$(version)_$(1)_$(2).zip $(appname)$(3) && rm $(appname)$(3)
>>>>>>> master

.PHONY: build

all: windows darwin linux

clean:
	rm -rf build/

##### LINUX BUILDS #####
linux: build/$(appname)_$(version)_linux_arm.tar.gz build/$(appname)_$(version)_linux_arm64.tar.gz build/$(appname)_$(version)_linux_386.tar.gz build/$(appname)_$(version)_linux_amd64.tar.gz

build/$(appname)_$(version)_linux_386.tar.gz:
	$(call build,linux,386,)
	$(call tar,linux,386)

build/$(appname)_$(version)_linux_amd64.tar.gz:
	$(call build,linux,amd64,)
	$(call tar,linux,amd64)

build/$(appname)_$(version)_linux_arm.tar.gz:
	$(call build,linux,arm,)
	$(call tar,linux,arm)

build/$(appname)_$(version)_linux_arm64.tar.gz:
	$(call build,linux,arm64,)
	$(call tar,linux,arm64)

##### DARWIN (MAC) BUILDS #####
darwin: build/$(appname)_$(version)_darwin_amd64.tar.gz build/$(appname)_$(version)_darwin_arm64.tar.gz

build/$(appname)_$(version)_darwin_arm64.tar.gz:
	$(call build,darwin,arm64,)
	$(call tar,darwin,arm64)

build/$(appname)_$(version)_darwin_amd64.tar.gz:
	$(call build,darwin,amd64,)
	$(call tar,darwin,amd64)

##### WINDOWS BUILDS #####
windows: build/$(appname)_$(version)_windows_386.zip build/$(appname)_$(version)_windows_amd64.zip

build/$(appname)_$(version)_windows_386.zip:
	$(call build,windows,386,.exe)
	$(call zip,windows,386,.exe)

build/$(appname)_$(version)_windows_amd64.zip:
	$(call build,windows,amd64,.exe)
	$(call zip,windows,amd64,.exe)

install: go.sum
	@echo "--> Installing git-remote-gitopia"
	@go install -mod=readonly $(BUILD_FLAGS) ./cmd/git-remote-gitopia
<<<<<<< HEAD
	@go install -mod=readonly $(BUILD_FLAGS) ./cmd/git-gitopia-keys
	mv $(GOBIN)/git-gitopia-keys $(GOBIN)/git-gitopia
=======
>>>>>>> master

go.sum: go.mod
	@echo "--> Ensure dependencies have not been modified"
	GO111MODULE=on go mod verify
