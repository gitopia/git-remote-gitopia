package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/version"
)

const (
	// !!NOTE!! keep this same as remote helper app name
	AppName              = "git-remote-gitopia"
	AccountAddressPrefix = "gitopia"
)

func main() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, new(client.Context))
	version.Name = AppName // os keyring service name is same as version name

	err := RootCommand().ExecuteContext(ctx)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return
	}
	fmt.Fprintf(os.Stdout, "Done\n")
}
