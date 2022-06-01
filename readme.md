# git-remote-gitopia

git remote helper for [gitopia](https://gitopia.org/)

## Installation

You need to install `git-remote-gitopia` helper so that your git command line can understand `gitopia://` transport.

```
curl https://get.gitopia.com | bash
```

If you get the following error

```
mv: rename ./git-remote-gitopia to /usr/local/bin/git-remote-gitopia: Permission denied
============
Error: mv failed
```

You would need root permission to move the binary to `/usr/local/bin`

```
sudo mv /tmp/tmpinstalldir/git-remote-gitopia /usr/local/bin/
```

## Building

Building `git-remote-gitopia` requires [Go 1.16+](https://golang.org/dl/).

```
make install
```

By default, make builds with production configurations. If you want to build with `dev` or `testing` configurations, set an env variable `GITOPIA_ENV` with respective value

For example, you can build with dev configurations using the following command
```
export GITOPIA_ENV=dev && make install
```

## Usage

`git-remote-gitopia` will be implicitly called when git encounters `gitopia://` remote.

For pushing git repositories to gitopia, you would require a gitopia wallet with sufficient tokens and you need to configure an environment variable with the location of your wallet.

```sh
export GITOPIA_WALLET=/path/to/wallet.json
```

## Troubleshooting

If you encounter the following error after installation, make sure to add `$GOBIN` to your `$PATH`

```sh
git: 'remote-gitopia' is not a git command. See 'git --help'
```
