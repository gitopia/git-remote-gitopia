module github.com/gitopia/git-remote-gitopia

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.45.1
	github.com/gitopia/gitopia v0.13.1-0.20220718095620-28278975e23b
	github.com/go-git/go-git/v5 v5.4.2
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	github.com/tendermint/starport v0.19.2
	google.golang.org/grpc v1.46.2
)

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1

replace github.com/keybase/go-keychain => github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4
