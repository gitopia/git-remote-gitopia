module github.com/gitopia/git-remote-gitopia

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.45.0
	github.com/gitopia/gitopia v0.12.1-0.20220302171918-ccf7c17f9817
	github.com/go-git/go-git/v5 v5.4.2
	google.golang.org/grpc v1.45.0
)

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1

replace github.com/keybase/go-keychain => github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4
