package api

import (
	"net/http"
	"time"

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
		return ""
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

func SetConfiguredGitServerHost(host string) error {
	cmd := core.GitCommand("git", "config", "--global", "gitopia.gitServerHost", host)
	return cmd.Run()
}
