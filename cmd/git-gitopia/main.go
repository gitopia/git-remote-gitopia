package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	AccountAddressPrefix = "gitopia"
)

func main() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, &client.Context{})
	cmd := &cobra.Command{
		Use:               "gitopia",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			conf := sdk.GetConfig()
			conf.SetBech32PrefixForAccount(AccountAddressPrefix, AccountAddressPrefix+sdk.PrefixPublic)
			conf.Seal()
			initClientCtx := client.Context{}.
				WithInput(os.Stdin)
			return client.SetCmdClientContext(cmd, initClientCtx)
		},
	}
	cmd.AddCommand(keys.Commands("."))
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return
	}
	fmt.Fprintf(os.Stdout, "Done\n")
}
