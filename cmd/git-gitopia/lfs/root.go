package lfs

import (
	"github.com/spf13/cobra"
)

func Commands() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "lfs",
		Short:             "Configure the lfsconfig for the gitopia remote",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		},
	}
	cmd.AddCommand(InitCommand())

	return cmd
}
