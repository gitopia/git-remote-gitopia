package main

import "errors"

var (
	// Gitopia key is not configured in gitconfig
	ErrGitopiaKeyNotConfigured = errors.New("gitopia key not configured")
)
