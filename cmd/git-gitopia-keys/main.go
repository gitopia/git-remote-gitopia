package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/spf13/cobra"
)

func main(){
	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, &client.Context{})
	cmd := &cobra.Command{
		Use:               "git-gitopia",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
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