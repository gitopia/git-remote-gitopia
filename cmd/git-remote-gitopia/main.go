package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	core "github.com/gitopia/git-remote-gitopia/core"
)

const (
	GITOPIA_PREFIX = "gitopia://"
)

func Main(args []string, reader io.Reader, writer io.Writer, logger *log.Logger) error {
	var remoteUserId, remoteRepositoryName string

	if len(args) < 3 {
		return fmt.Errorf("Usage: git-remote-gitopia remote-name url")
	}

	remoteName := args[2]
	if strings.HasPrefix(remoteName, GITOPIA_PREFIX) {
		remoteUserId = remoteName[len(GITOPIA_PREFIX) : len(GITOPIA_PREFIX)+46]
		remoteRepositoryName = remoteName[len(GITOPIA_PREFIX)+47:]
	} else {
		return fmt.Errorf("Invalid remote url")
	}

	remote, err := core.NewRemote(&GitopiaHandler{remoteUserId: remoteUserId, remoteRepositoryName: remoteRepositoryName}, reader, writer, logger)
	if err != nil {
		return err
	}
	defer remote.Handler.Finish(remote)

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
