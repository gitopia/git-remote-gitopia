package core

import (
	"errors"
	"fmt"
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

func GitCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
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
	if !strings.HasPrefix(remoteURL, GITOPIA_PREFIX) {
		return "", "", fmt.Errorf("invalid gitopia remote url: must start with '%s', got '%s'", GITOPIA_PREFIX, remoteURL)
	}

	s := strings.TrimPrefix(remoteURL, GITOPIA_PREFIX)
	sp := strings.Split(s, "/")

	if len(sp) != 2 {
		return "", "", fmt.Errorf("invalid gitopia remote url: expected format 'gitopia://user/repository', got '%s' (found %d parts after prefix)", remoteURL, len(sp))
	}

	remoteUserId = sp[0]
	remoteRepositoryName = sp[1]

	if remoteUserId == "" {
		return "", "", fmt.Errorf("invalid gitopia remote url: user ID cannot be empty in '%s'", remoteURL)
	}

	if remoteRepositoryName == "" {
		return "", "", fmt.Errorf("invalid gitopia remote url: repository name cannot be empty in '%s'", remoteURL)
	}

	// Try to parse as bech32 address first
	_, err = sdk.AccAddressFromBech32(remoteUserId)
	if err != nil {
		// If not a valid bech32 address, validate as username
		if len(remoteUserId) < 3 {
			return "", "", fmt.Errorf("invalid gitopia remote url: user ID '%s' is too short (minimum 3 characters)", remoteUserId)
		}
		if len(remoteUserId) > 39 {
			return "", "", fmt.Errorf("invalid gitopia remote url: user ID '%s' is too long (maximum 39 characters)", remoteUserId)
		}
		
		valid, regexErr := regexp.MatchString("^[a-zA-Z0-9]+(?:[-]?[a-zA-Z0-9])*$", remoteUserId)
		if regexErr != nil {
			return "", "", fmt.Errorf("invalid gitopia remote url: error validating user ID '%s': %v", remoteUserId, regexErr)
		}
		if !valid {
			return "", "", fmt.Errorf("invalid gitopia remote url: user ID '%s' contains invalid characters (only alphanumeric and hyphens allowed)", remoteUserId)
		}
	}

	return remoteUserId, remoteRepositoryName, nil
}
