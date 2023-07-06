package lfs

import (
	"github.com/spf13/cobra"
)

var Commands = &cobra.Command{
	Use:               "lfs",
	Short:             "Configure the lfsconfig for the gitopia remote",
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
	},
}
