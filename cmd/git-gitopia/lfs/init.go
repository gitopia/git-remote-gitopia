package lfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/gitopia/git-remote-gitopia/config"
	"github.com/gitopia/git-remote-gitopia/core"
	gitopiatypes "github.com/gitopia/gitopia/v2/x/gitopia/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var initCmd = &cobra.Command{
	Use:   "init <remote_name>",
	Short: "Initialize the lfsconfig for the gitopia remote",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := path.Dir(os.Getenv("GIT_DIR"))
		if dir == "" {
			return errors.New("not a git repository")
		}

		lfsConfigPath := path.Join(dir, ".lfsconfig")
		if _, err := os.Stat(lfsConfigPath); os.IsNotExist(err) {
			c, buf := core.GitCommand("git", "remote", "get-url", args[0])
			if err := c.Start(); err != nil {
				return err
			}

			output, err := io.ReadAll(buf)
			if err != nil {
				return err
			}

			if err := c.Wait(); err != nil {
				return err
			}

			remoteURL := strings.TrimSpace(string(output))

			remoteUserId, remoteRepositoryName, err := core.ValidateGitopiaRemoteURL(string(remoteURL))
			if err != nil {
				return err
			}

			interfaceRegistry := codectypes.NewInterfaceRegistry()

			grpcConn, err := grpc.Dial(config.GRPCHost,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithDefaultCallOptions(grpc.ForceCodec(codec.NewProtoCodec(interfaceRegistry).GRPCCodec())),
			)
			if err != nil {
				return err
			}
			defer grpcConn.Close()

			queryClient := gitopiatypes.NewQueryClient(grpcConn)

			// Get RepositoryId
			res, err := queryClient.AnyRepository(context.Background(), &gitopiatypes.QueryGetAnyRepositoryRequest{
				Id:             remoteUserId,
				RepositoryName: remoteRepositoryName,
			})
			if err != nil {
				return err
			}

			remoteRepository := *res.Repository
			lfsURL := fmt.Sprintf("%v/%v.git", config.GitServerHost, remoteRepository.Id)

			args := []string{
				"config",
				fmt.Sprintf("--file=%s", lfsConfigPath),
				"lfs.url",
				lfsURL,
			}

			cmd, _ := core.GitCommand("git", args...)
			if err := cmd.Run(); err != nil {
				return errors.Wrap(err, "error creating .lfsconfig")
			}

			return nil
		}

		return errors.New(".lfsconfig file already exists")
	},
}

func init() {
	Commands.AddCommand(initCmd)
}
