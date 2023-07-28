# Change Log

All notable changes will be documented here.

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
