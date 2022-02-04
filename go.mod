module github.com/gitopia/git-remote-gitopia

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.44.3
	github.com/gitopia/gitopia v0.10.2-0.20220107024322-d6707fd7776f
	github.com/go-git/go-git/v5 v5.4.2
	google.golang.org/grpc v1.43.0
)

replace google.golang.org/grpc => google.golang.org/grpc v1.33.2

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1
