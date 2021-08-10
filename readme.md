# git-remote-gitopia

git remote helper for [gitopia](https://gitopia.org/)

## Installation

Installing `git-remote-gitopia` requires [Go 1.16+](https://golang.org/dl/).

```
make install
```

## Usage

`git-remote-gitopia` will be implicitly called when git encounters `gitopia://` remote.

For pushing git repositories to gitopia, you would require a gitopia wallet with sufficient tokens and you need to configure an environment variable with the location of your wallet.

```sh
export GITOPIA_WALLET=/path/to/wallet.json
```
