module github.com/gitopia/git-remote-gitopia

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.42.4 // indirect
	github.com/gitopia/gitopia v0.0.0-20210809131017-9aa8001911af
	github.com/go-git/go-git/v5 v5.4.2
	google.golang.org/grpc v1.38.0
)

replace google.golang.org/grpc => google.golang.org/grpc v1.33.2

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1
