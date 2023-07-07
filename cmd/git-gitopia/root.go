package main

import (
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/gitopia/git-remote-gitopia/cmd/git-gitopia/lfs"
	"github.com/gitopia/gitopia-go"
	"github.com/spf13/cobra"
)

func RootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "gitopia",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return gitopia.CommandInit(cmd, AppName)
		},
	}
	cmd.AddCommand(keys.Commands("."))
	cmd.AddCommand(lfs.Commands())

	return cmd
}
