package main

import (
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gitopia/git-remote-gitopia/cmd/git-gitopia/lfs"
	"github.com/spf13/cobra"
)

func RootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "gitopia",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			conf := sdk.GetConfig()
			conf.SetBech32PrefixForAccount(AccountAddressPrefix, AccountAddressPrefix+sdk.PrefixPublic)
			conf.Seal()

			registry := codectypes.NewInterfaceRegistry()
			cryptocodec.RegisterInterfaces(registry)
			marshaler := codec.NewProtoCodec(registry)

			initClientCtx := client.GetClientContextFromCmd(cmd).
				WithCodec(marshaler).
				WithInterfaceRegistry(registry).
				WithInput(os.Stdin)

			// sets global flags for keys subcommand
			return client.SetCmdClientContextHandler(initClientCtx, cmd)
		},
	}
	cmd.AddCommand(keys.Commands("."))
	cmd.AddCommand(lfs.Commands())

	return cmd
}
