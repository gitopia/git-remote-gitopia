package core

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	GITOPIA_PREFIX = "gitopia://"
)

var ErrInvalidGitopiaRemoteURL = errors.New("invalid gitopia remote url")

func GetLocalDir() (string, error) {
	localdir := path.Join(os.Getenv("GIT_DIR"))
	if localdir == "" {
		return "", nil
	}

	if err := os.MkdirAll(localdir, 0755); err != nil {
		return "", err
	}
	return localdir, nil
}

func GitCommand(name string, args ...string) (*exec.Cmd, io.Reader) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func CleanUpProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	process := cmd.Process
	if process != nil {
		process.Signal(os.Kill)
	}

	go cmd.Wait()
}

func ValidateGitopiaRemoteURL(remoteURL string) (remoteUserId string, remoteRepositoryName string, err error) {
	if strings.HasPrefix(remoteURL, GITOPIA_PREFIX) {
		s := strings.TrimPrefix(remoteURL, GITOPIA_PREFIX)
		sp := strings.Split(s, "/")

		if len(sp) != 2 {
			return "", "", ErrInvalidGitopiaRemoteURL
		}
		remoteUserId = sp[0]
		remoteRepositoryName = sp[1]

		_, err := sdk.AccAddressFromBech32(remoteUserId)
		if err != nil {
			if len(remoteUserId) < 3 || len(remoteUserId) > 39 {
				return "", "", ErrInvalidGitopiaRemoteURL
			}
			valid, err := regexp.MatchString("^[a-zA-Z0-9]+(?:[-]?[a-zA-Z0-9])*$", remoteUserId)
			if err != nil {
				return "", "", ErrInvalidGitopiaRemoteURL
			}
			if !valid {
				return "", "", ErrInvalidGitopiaRemoteURL
			}
		}
		return remoteUserId, remoteRepositoryName, nil
	}

	return "", "", ErrInvalidGitopiaRemoteURL
}
