package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gitopia/git-remote-gitopia/core/wallet"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	actionGet   = "get"
	actionStore = "store"
	actionErase = "erase"
)

func main() {
	var action string
	//nolint:gomnd // Just a count of cli arguments, no need to make a constant
	if len(os.Args) == 2 {
		action = os.Args[1]
	}

	switch action {

	case actionGet:
		handleGetAction()

	case actionErase, actionStore:
		// Defined action but unsupported by this provider
		return

	default:
		log.Fatalf("Unsupported action %q", action)

	}
}

func handleGetAction() {
	values, err := readInput(os.Stdin)
	if err != nil {
		log.WithError(err).Fatal("Unable to parse input values")
	}

	if proto := values["protocol"]; proto != "https" {
		log.Debugf("Unsupported protocol %q", proto)
		return
	}

	wallet, err := wallet.InitWallet(nil, nil)
	if err != nil {
		log.Debugf(err.Error())
		return
	}

	signature, err := wallet.SignData([]byte(values["host"]))
	if err != nil {
		log.Debugf(err.Error())
		return
	}

	values["username"] = wallet.Address()
	values["password"] = signature

	fmt.Fprintln(os.Stdout, strings.Join(MapToList(values), "\n"))
}

func readInput(input io.Reader) (map[string]string, error) {
	var (
		lines   []string
		scanner = bufio.NewScanner(input)
	)

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())

		if text == "" {
			break
		}

		lines = append(lines, text)
	}

	return ListToMap(lines), errors.Wrap(scanner.Err(), "Unable to read input")
}
