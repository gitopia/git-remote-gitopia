package config

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	AppName              = "git-remote-gitopia"
	gitopiaConfigSection = "gitopia"
)

func gitConfigGet(key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", fmt.Sprintf("gitopia.%s", key))
	stdout, err := cmd.Output()

	if err != nil {
		return "", err
	}

	res := strings.TrimSpace(string(stdout))
	return res, nil
}

func LoadGitConfig() error {
	if res, err := gitConfigGet("chainId"); err == nil {
		ChainId = res
	}
	if res, err := gitConfigGet("grpcHost"); err == nil {
		GRPCHost = res
	}
	if res, err := gitConfigGet("gitServerHost"); err == nil {
		GitServerHost = res
	}
	if res, err := gitConfigGet("tmAddr"); err == nil {
		TmAddr = res
	}
	if res, err := gitConfigGet("gasPrices"); err == nil {
		GasPrices = res
	}
	if res, err := gitConfigGet("feeGranter"); err == nil {
		FeeGranterAddr = res
	}
	if res, err := gitConfigGet("denom"); err == nil {
		Denom = res
	}
	return nil
}
