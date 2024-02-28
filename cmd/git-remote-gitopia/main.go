package main

import (
	"fmt"
	"io"
	"log"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gitopia/git-remote-gitopia/config"
	core "github.com/gitopia/git-remote-gitopia/core"
)

func Main(args []string, reader io.Reader, writer io.Writer, logger *log.Logger) error {
	config.LoadGitConfig()

	conf := sdk.GetConfig()
	conf.SetBech32PrefixForAccount(AccountAddressPrefix, AccountPubKeyPrefix)
	// cannot seal the config
	// cosmos client sets address prefix for each broadcasttx API call. probably a bug
	// conf.Seal()

	if len(args) < 3 {
		return fmt.Errorf("Usage: git-remote-gitopia remote-name url")
	}

	remoteUserId, remoteRepositoryName, err := core.ValidateGitopiaRemoteURL(args[2])
	if err != nil {
		return err
	}

	remote, err := core.NewRemote(&GitopiaHandler{remoteUserId: remoteUserId, remoteRepositoryName: remoteRepositoryName}, reader, writer, logger)
	if err != nil {
		return err
	}

	if err := remote.ProcessCommands(); err != nil {
		return err
	}

	return nil
}

func main() {
	if err := Main(os.Args, os.Stdin, os.Stdout, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\x1b[K")
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Done\n")
}
