package api

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/gitopia/git-remote-gitopia/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func getGRPCHosts() []string {
	return []string{
		"gitopia-grpc.polkachu.com:11390",
	}
}

func CheckLiveness(host string) bool {
	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false
	}
	defer conn.Close()

	client := tmservice.NewServiceClient(conn)
	_, err = client.GetNodeInfo(context.Background(), &tmservice.GetNodeInfoRequest{})
	return err == nil
}

func checkLatency(host string) time.Duration {
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

func GetBestGRPCHost() string {
	hosts := getGRPCHosts()
	bestHost := hosts[0]
	bestLatency := time.Hour

	for _, host := range hosts {
		latency := checkLatency(host)
		if latency < bestLatency {
			bestHost = host
			bestLatency = latency
		}
	}
	return bestHost
}

func GetConfiguredGRPCHost() string {
	cmd := exec.Command("git", "config", "--get", "gitopia.grpcHost")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func SetConfiguredGRPCHost(host string) error {
	cmd := core.GitCommand("git", "config", "--global", "gitopia.grpcHost", host)
	return cmd.Run()
}
