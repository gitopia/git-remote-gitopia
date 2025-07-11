package lfs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/gitopia/git-remote-gitopia/config"
	"github.com/gitopia/git-remote-gitopia/core"
	"github.com/gitopia/git-remote-gitopia/core/api"
	gitopiatypes "github.com/gitopia/gitopia/v6/x/gitopia/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func InitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <remote_name>",
		Short: "Initialize the lfsconfig for the gitopia remote",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := path.Dir(os.Getenv("GIT_DIR"))
			if dir == "" {
				return errors.New("not a git repository")
			}

			lfsConfigPath := path.Join(dir, ".lfsconfig")
			if _, err := os.Stat(lfsConfigPath); !errors.Is(err, fs.ErrNotExist) {
				return errors.New(".lfsconfig file already exists")

			}

			c := exec.Command("git", "remote", "get-url", args[0])
			output, err := c.Output()
			if err != nil {
				return errors.Wrap(err, "error reading the remote url")
			}

			remoteURL := strings.TrimSpace(string(output))
			remoteUserId, remoteRepositoryName, err := core.ValidateGitopiaRemoteURL(string(remoteURL))
			if err != nil {
				return err
			}

			interfaceRegistry := codectypes.NewInterfaceRegistry()

			grpcHost, _ := config.GitConfigGet(config.GitopiaConfigGRPCHostOption)
			if grpcHost == "" || !api.CheckGRPCHostLiveness(grpcHost) {
				provider := api.GetBestApiProvider()
				if err := api.SetConfiguredGRPCHost(provider.GRPCHost); err != nil {
					return err
				}
			}
			gitServerHost, _ := config.GitConfigGet(config.GitopiaConfigGitServerHostOption)
			if gitServerHost == "" {
				gitServerHost = api.GetBestGitServerHost(grpcHost)
				if gitServerHost != "" {
					if err := api.SetConfiguredGitServerHost(gitServerHost); err != nil {
						return err
					}
				}
			}

			grpcConn, err := grpc.Dial(grpcHost,
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
			gitServerHost, _ = config.GitConfigGet(config.GitopiaConfigGitServerHostOption)
			if gitServerHost == "" {
				gitServerHost = config.GitServerHost
			}
			lfsURL := fmt.Sprintf("%v/%v.git", gitServerHost, remoteRepository.Id)

			c = core.GitCommand("git", "config",
				fmt.Sprintf("--file=%s", lfsConfigPath),
				"lfs.url",
				lfsURL)
			if err := c.Run(); err != nil {
				return errors.Wrap(err, "error creating .lfsconfig")
			}

			return nil
		},
	}
	return cmd
}
