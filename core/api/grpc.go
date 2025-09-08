package api

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/gitopia/git-remote-gitopia/core"
	storagetypes "github.com/gitopia/gitopia/v6/x/storage/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func CheckGRPCHostLiveness(host string) bool {
	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false
	}
	defer conn.Close()

	client := tmservice.NewServiceClient(conn)
	res, err := client.GetSyncing(context.Background(), &tmservice.GetSyncingRequest{})
	if err != nil {
		return false
	}

	if res.Syncing {
		return false
	}

	return true
}

func checkGRPCHostLatency(host string) time.Duration {
	start := time.Now()
	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return time.Hour
	}
	defer conn.Close()

	client := tmservice.NewServiceClient(conn)
	_, err = client.GetNodeInfo(context.Background(), &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return time.Hour
	}
	return time.Since(start)
}

func SetConfiguredGRPCHost(host string) error {
	cmd := core.GitCommand("git", "config", "--global", "gitopia.grpcHost", host)
	return cmd.Run()
}

func GetActiveStorageProviders(host string) []storagetypes.Provider {
	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil
	}
	defer conn.Close()

	client := storagetypes.NewQueryClient(conn)
	res, err := client.ActiveProviders(context.Background(), &storagetypes.QueryActiveProvidersRequest{})
	if err != nil {
		return nil
	}
	return res.Providers
}
