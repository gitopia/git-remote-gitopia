package api

import (
	"context"
	"time"

	clienthttp "github.com/cometbft/cometbft/rpc/client/http"
	jsonrpcclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/gitopia/git-remote-gitopia/core"
)

func CheckRPCHostLiveness(host string) bool {
	httpClient, err := jsonrpcclient.DefaultHTTPClient(host)
	if err != nil {
		return false
	}
	httpClient.Timeout = time.Duration(5) * time.Second
	c, err := clienthttp.NewWithClient(host, "/websocket", httpClient)
	if err != nil {
		return false
	}

	res, err := c.Status(context.Background())
	if err != nil {
		return false
	}
	if res.SyncInfo.CatchingUp {
		return false
	}

	return true
}

func SetConfiguredTmAddr(addr string) error {
	cmd := core.GitCommand("git", "config", "--global", "gitopia.tmAddr", addr)
	return cmd.Run()
}
