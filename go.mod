module github.com/gitopia/git-remote-gitopia

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.46.1
	github.com/gitopia/gitopia v1.0.0-rc.2
	github.com/go-git/go-git/v5 v5.4.2
	github.com/ignite/cli v0.24.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.5.0
	google.golang.org/grpc v1.49.0
)

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1
