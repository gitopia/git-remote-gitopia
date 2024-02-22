package config

import (
	"cosmossdk.io/errors"
	goGitConfig "github.com/go-git/go-git/v5/config"
)

const (
	AppName              = "git-remote-gitopia"
	gitopiaConfigSection = "gitopia"

	gitopiaConfigChainid       = "chainid"
	gitopiaConfigGrpchost      = "grpchost"
	gitopiaConfigGitserverhost = "gitserverhost"
	gitopiaConfigTmaddr        = "tmaddr"
	gitopiaConfigGasprices     = "gasprices"
	gitopiaConfigFeegranter    = "feegranter"
	gitopiaConfigDenom         = "denom"
)

func LoadGitConfig() error {
	conf, err := goGitConfig.LoadConfig(goGitConfig.GlobalScope)
	if err != nil {
		return errors.Wrap(err, "error loading git config")
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigChainid) {
		ChainId = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigChainid)
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigGrpchost) {
		GRPCHost = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigGrpchost)
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigGitserverhost) {
		GitServerHost = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigGitserverhost)
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigTmaddr) {
		TmAddr = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigTmaddr)
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigGasprices) {
		GasPrices = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigGasprices)
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigFeegranter) {
		FeeGranterAddr = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigFeegranter)
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigDenom) {
		Denom = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigDenom)
	}
	return nil
}
