package wallet

import (
	"errors"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc"
)

type secretType int

const (
	UNKNOWN secretType = iota
	ENV_VAR
	LEDGER
	KEYRING_BACKEND
	GITHUB_SEC
)

type Wallet interface {
	SignData(data []byte) (string, error)
	SignAndBroadcast(grpcConn *grpc.ClientConn, msgs []sdk.Msg) error
	Type() secretType
	Address() string
}

func InitWallet() (Wallet, error) {
	wallet, err := InitOSKeyringWallet()
	if errors.Is(err, ErrGitopiaKeyNotConfigured) {
		wallet, err = InitGitopiaWallet()
		if err != nil {
			wallet, err = InitLedgerWallet()
			if err != nil {
				return nil, fmt.Errorf("fatal: Gitopia wallet is not configured! Set gitopia key or use Ledger")
			}
		}
	} else if err != nil {
		return nil, fmt.Errorf("fatal: Cannot access the gitopia key from the OS keyring, %s", err.Error())
	}

	return wallet, nil
}
