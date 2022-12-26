package core

import (
	"os"
	"path"
)

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
