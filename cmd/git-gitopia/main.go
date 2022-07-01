package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	AppName              = "gitopia"
	AccountAddressPrefix = "gitopia"
)

func main() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, new(client.Context))
	version.Name = AppName // os keyring service name is same as version name 
	cmd := &cobra.Command{
		Use:               "gitopia",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			conf := sdk.GetConfig()
			conf.SetBech32PrefixForAccount(AccountAddressPrefix, AccountAddressPrefix+sdk.PrefixPublic)
			conf.Seal()
			initClientCtx := client.Context{}.WithInput(os.Stdin)
			// sets global flags for keys subcommand
			return client.SetCmdClientContextHandler(initClientCtx, cmd)
		},
	}
	cmd.AddCommand(keys.Commands("."))
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return
	}
	fmt.Fprintf(os.Stdout, "Done\n")
}
