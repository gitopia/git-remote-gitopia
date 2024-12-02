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

Building `git-remote-gitopia` requires [Go 1.18+](https://golang.org/dl/).

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

## Custom Endpoint Configuration Example

### Testnet
```
git config --global gitopia.chainId "gitopia"
git config --global gitopia.grpcHost "localhost:9090"
git config --global gitopia.gitServerHost "http://localhost:5001"
git config --global gitopia.tmAddr "http://localhost:26657"
git config --global gitopia.gasPrices "0.001ulore"
git config --global gitopia.feeGranter ""
git config --global gitopia.denom "ulore"
```

### Mainnet
```
git config --global gitopia.chainId "gitopia"
git config --global gitopia.grpcHost "grpc.gitopia.com:9090"
git config --global gitopia.gitServerHost "https://server.gitopia.com"
git config --global gitopia.tmAddr "https://rpc.gitopia.com:443"
git config --global gitopia.gasPrices "0.001ulore"
git config --global gitopia.feeGranter "gitopia13ashgc6j5xle4m47kqyn5psavq0u3klmscfxql"
git config --global gitopia.denom "ulore"
```

## Troubleshooting

If you encounter the following error after installation, make sure to add `$GOBIN` to your `$PATH`

```sh
git: 'remote-gitopia' is not a git command. See 'git --help'
```

## Contributing

Gitopia is an open source project and contributions from community are always welcome. Discussion and development of Gitopia majorly take place on the Gitopia via issues and proposals -- everyone is welcome to post bugs, feature requests, comments and pull requests to Gitopia. (read [Contribution Guidelines](CONTRIBUTING.md) and [Coding Guidelines](CodingGuidelines.md).
