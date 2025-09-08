package config

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	AppName                          = "git-remote-gitopia"
	GitopiaConfigSection             = "gitopia"
	GitopiaConfigChainIdOption       = "chainId"
	GitopiaConfigGRPCHostOption      = "grpcHost"
	GitopiaConfigTmAddrOption        = "tmAddr"
	GitopiaConfigGitServerHostOption = "gitServerHost"
	GitopiaConfigKeyOption           = "key"
	GitopiaConfigBackendOption       = "backend"
	GitopiaConfigGasPricesOption     = "gasPrices"
	GitopiaConfigFeeGranterOption    = "feeGranter"
	GitopiaConfigDenomOption         = "denom"
)

func GitConfigGet(key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", fmt.Sprintf("gitopia.%s", key))
	stdout, err := cmd.Output()

	if err != nil {
		return "", err
	}

	res := strings.TrimSpace(string(stdout))
	return res, nil
}

func LoadGitConfig() error {
	if res, err := GitConfigGet(GitopiaConfigChainIdOption); err == nil {
		ChainId = res
	}
	if res, err := GitConfigGet(GitopiaConfigGasPricesOption); err == nil {
		GasPrices = res
	}
	if res, err := GitConfigGet(GitopiaConfigFeeGranterOption); err == nil {
		FeeGranterAddr = res
	}
	if res, err := GitConfigGet(GitopiaConfigDenomOption); err == nil {
		Denom = res
	}
	return nil
}
