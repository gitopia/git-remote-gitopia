package api

import (
	"net/http"
	"time"

	"github.com/gitopia/git-remote-gitopia/config"
	"github.com/gitopia/git-remote-gitopia/core"
)

func CheckGitServerHostLiveness(host string) bool {
	res, err := http.Get(host + "/healthz")
	if err != nil {
		return false
	}
	defer res.Body.Close()

	return res.StatusCode == http.StatusOK
}

func checkHttpHostLatency(host string) time.Duration {
	start := time.Now()
	_, err := http.Get(host)
	if err != nil {
		return time.Hour
	}
	return time.Since(start)
}

func GetBestGitServerHost(grpcHost string) string {
	providers := GetActiveStorageProviders(grpcHost)
	if len(providers) == 0 {
		// No active providers found, use fallback provider if configured
		return GetFallbackProviderHost(grpcHost)
	}

	var bestHost string
	bestLatency := time.Hour

	for _, p := range providers {
		latency := checkHttpHostLatency(p.ApiUrl)
		if latency < bestLatency {
			bestHost = p.ApiUrl
			bestLatency = latency
		}
	}

	return bestHost
}

// GetFallbackProviderHost queries for the fallback provider's API URL using the given gRPC host
func GetFallbackProviderHost(grpcHost string) string {
	if config.FallbackProvider != "" {
		provider := GetStorageProvider(grpcHost, config.FallbackProvider)
		if provider.ApiUrl != "" {
			return provider.ApiUrl
		}
	}
	return ""
}

func SetConfiguredGitServerHost(host string) error {
	cmd := core.GitCommand("git", "config", "--global", "gitopia.gitServerHost", host)
	return cmd.Run()
}
