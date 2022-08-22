package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	core "github.com/gitopia/git-remote-gitopia/core"
)

const (
	GITOPIA_PREFIX = "gitopia://"
)

func Main(args []string, reader io.Reader, writer io.Writer, logger *log.Logger) error {
	var remoteUserId, remoteRepositoryName string

	conf := sdk.GetConfig()
	conf.SetBech32PrefixForAccount(AccountAddressPrefix, AccountPubKeyPrefix)
	// cannot seal the config
	// cosmos client sets address prefix for each broadcasttx API call. probably a bug
	// conf.Seal()

	if len(args) < 3 {
		return fmt.Errorf("Usage: git-remote-gitopia remote-name url")
	}

	remoteName := args[2]
	if strings.HasPrefix(remoteName, GITOPIA_PREFIX) {
		s := strings.TrimPrefix(remoteName, GITOPIA_PREFIX)
		sp := strings.Split(s, "/")

		if len(sp) != 2 {
			return fmt.Errorf("Invalid remote url")
		}
		remoteUserId = sp[0]
		remoteRepositoryName = sp[1]

		if len(remoteUserId) == 46 { // gitopia address
			_, err := sdk.AccAddressFromBech32(remoteUserId)
			if err != nil {
				return fmt.Errorf("Invalid remote url")
			}
		} else { // username
			if len(remoteUserId) < 3 || len(remoteUserId) > 39 {
				return fmt.Errorf("Invalid remote url")
			}

			valid, err := regexp.MatchString("^[a-zA-Z0-9]+(?:[-]?[a-zA-Z0-9])*$", remoteUserId)
			if err != nil {
				return fmt.Errorf("Invalid remote url")
			}
			if !valid {
				return fmt.Errorf("Invalid remote url")
			}
		}
	} else {
		return fmt.Errorf("Invalid remote url")
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
