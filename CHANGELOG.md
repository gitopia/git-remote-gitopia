# Change Log

All notable changes will be documented here.

## [v1.10.0] - 2024-12-02

- upgrade gitopia version to v5.1.0
- upgrade gitopia-go version to v0.6.2
- update the push permissions logic for dao repositories

## [v1.9.1] - 2024-08-31

- fix offchain proto warning
- remove websocket connection with rpc servers
- check sync status in rpc provider liveness test

## [v1.9.0] - 2024-08-20

- upgrade cosmos-sdk to v0.47.13, gitopia to v4.0.0 and gitopia-go to v0.6.0
- automatic selection of grpc host with the least latency
- set gas adjustment to 1.8

## [v1.8.0] - 2024-05-31

- Support custom configuration for api and other parameters
- Reduce the size of release binaries
- Fix ledger wallet auth

## [v1.7.0] - 2023-07-29

- Command to initialize lfs config
  - `git gitopia lfs init <remote_name>`
- Performance improvements in fetch
- Show progress in fetch and push
- Give priority to wallet balance over feegrant

## [v1.6.0] - 2023-06-07

- fix os keyring wallet in case of feegrant
- use gitopia-go instead of cosmosclient for os keyring wallet support

## [v1.5.0] - 2023-05-17

- git credential helper for git lfs
- Use git cli instead of go-git for fetch/push
- Common http basic auth for both git push and lfs
- Refactor wallets
- Support fee grant
- Upgrade gitopia version to v2.0.1

## [v1.4.0] - 2023-02-22

- Upgrade gitopia version to v1.3.0

## [v1.3.2] - 2022-12-26

- Allow invocation without `GIT_DIR` variable

## [v1.3.1] - 2022-11-15

- Fix node address in prod config

## [v1.3.0] - 2022-11-07

- Bump gitopia version to v1.2.0

## [v1.2.0] - 2022-11-02

- Bump gitopia version to v1.1.2

## [v1.1.0] - 2022-10-27

- Bump gitopia version to v1.1.0
- Support for Ledger Nano S plus
- Log wallet type and address

## [v1.0.0] - 2022-10-18

- Added `git gitopia keys` command to manage keys in OS keyring
- API changes for gitopia v1.0.0 and cosmos-sdk v0.46.2
- Support usernames and dao names in remote url
- Send auth token in git push request
