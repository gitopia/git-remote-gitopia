package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
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
	cmd := &cobra.Command{
		Use:               "gitopia",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			conf := sdk.GetConfig()
			conf.SetBech32PrefixForAccount(AccountAddressPrefix, AccountAddressPrefix+sdk.PrefixPublic)
			conf.Seal()

			registry := codectypes.NewInterfaceRegistry()
			cryptocodec.RegisterInterfaces(registry)
			kb, err := cmd.Flags().GetString(flags.FlagKeyringBackend)
			if err != nil {
				return err
			}
			kd, err := cmd.Flags().GetString(flags.FlagKeyringDir)
			if err != nil {
				return err
			}
			k, err := sdkkeyring.New(AppName, kb, kd, os.Stdin, codec.NewProtoCodec(registry))
			if err != nil {
				return err
			}

			initClientCtx := client.Context{}.WithInput(os.Stdin).WithKeyring(k)

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
