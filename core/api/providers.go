package api

import (
	"time"
)

type ProviderConfig struct {
	GRPCHost string
	TMAddr   string
}

var providers = []ProviderConfig{
	{
		GRPCHost: "gitopia.declab.pro:9017",
		TMAddr:   "https://gitopia.declab.pro:26625",
	},
	{
		GRPCHost: "gitopia-grpc.polkachu.com:11390",
		TMAddr:   "https://gitopia-rpc.polkachu.com:443",
	},
	{
		GRPCHost: "gitopia.grpc.m.stavr.tech:5123",
		TMAddr:   "http://gitopia.rpc.m.stavr.tech:51057",
	},
	{
		GRPCHost: "gitopia-rpc.stakeangle.com:41390",
		TMAddr:   "https://gitopia-rpc.stakeangle.com:443",
	},
}

func GetBestApiProvider() ProviderConfig {
	bestHost := providers[0]
	bestLatency := time.Hour

	for _, p := range providers {
		latency := checkGRPCHostLatency(p.GRPCHost)
		if latency < bestLatency && CheckRPCHostLiveness(p.TMAddr) {
			bestHost = p
			bestLatency = latency
		}
	}

	return bestHost
}
