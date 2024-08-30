package api

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/gitopia/git-remote-gitopia/core"
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
	_, err = client.GetNodeInfo(context.Background(), &tmservice.GetNodeInfoRequest{})
	return err == nil
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
