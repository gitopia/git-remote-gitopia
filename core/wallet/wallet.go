package wallet

import (
	"errors"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
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

// TODO: make options optional
func InitWallet(feegrantClient feegrant.QueryClient) (Wallet, error) {
	wallet, err := InitOSKeyringWallet(feegrantClient)
	if errors.Is(err, ErrGitopiaKeyNotConfigured) {
		wallet, err = InitGitopiaWallet(feegrantClient)
		if err != nil {
			wallet, err = InitLedgerWallet(feegrantClient)
			if err != nil {
				return nil, fmt.Errorf("fatal: Gitopia wallet is not configured! Set gitopia key or use Ledger")
			}
		}
	} else if err != nil {
		return nil, fmt.Errorf("fatal: Cannot access the gitopia key from the OS keyring, %s", err.Error())
	}

	return wallet, nil
}
