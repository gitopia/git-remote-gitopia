package wallet

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	"github.com/gitopia/git-remote-gitopia/config"
	glib "github.com/gitopia/gitopia-go"
	"github.com/gitopia/gitopia-go/logger"
	gitopia "github.com/gitopia/gitopia/v4/app"
	offchaintypes "github.com/gitopia/gitopia/v4/x/offchain/types"
	goGitConfig "github.com/go-git/go-git/v5/config"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

const (
	AppName                    = "git-remote-gitopia"
	AccountAddressPrefix       = "gitopia"
	gitopiaConfigSection       = "gitopia"
	gitopiaConfigKeyOption     = "key"
	gitopiaConfigBackendOption = "backend"
)

var (
	// Gitopia key is not configured in gitconfig
	ErrGitopiaKeyNotConfigured = errors.New("gitopia key not configured")
)

type keyringBackend struct {
	key     string
	backend string
	CC      glib.Client
}

func newKeyringBackend(k string, b string, c glib.Client) keyringBackend {
	return keyringBackend{
		key:     k,
		backend: b,
		CC:      c,
	}
}

type OSKeyring struct {
	kb      keyringBackend
	address string
	secType secretType
}

func InitOSKeyringWallet(bankClient banktypes.QueryClient, feegrantClient feegrant.QueryClient) (Wallet, error) {
	var key string
	var backend string

	conf, err := goGitConfig.LoadConfig(goGitConfig.GlobalScope)
	if err != nil {
		return nil, errors.Wrap(err, "error loading git config")
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigKeyOption) {
		key = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigKeyOption)
	} else {
		return nil, ErrGitopiaKeyNotConfigured
	}

	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigBackendOption) {
		backend = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigBackendOption)
	} else {
		backend = keyring.BackendOS // default to OS. same as cosmos keys subcommand
	}

	l := logrus.New()
	l.SetOutput(os.Stderr)
	ctx := logger.ContextWithValue(context.Background(), l)
	glib.WithAppName(AppName)
	glib.WithGasPrices(config.GasPrices)
	glib.WithGitopiaAddr(config.GRPCHost)
	glib.WithChainId(config.ChainId)
	glib.WithTmAddr(config.TmAddr)

	cc, err := glib.GetClientContextWithOptions(AppName, backend, key)
	if err != nil {
		return nil, errors.Wrap(err, "error creating cosmos client context")
	}

	if bankClient != nil && feegrantClient != nil {
		b, err := bankClient.Balance(context.Background(), &banktypes.QueryBalanceRequest{
			Address: cc.FromAddress.String(),
			Denom:   config.Denom,
		})
		if err != nil {
			return nil, errors.New("error querying for balance")
		}

		// Use fee grant only when balance is zero
		if b.Balance.Amount.IsZero() {
			fr, _ := feegrantClient.Allowance(context.Background(), &feegrant.QueryAllowanceRequest{
				Granter: config.FeeGranterAddr,
				Grantee: cc.FromAddress.String(),
			})
			if fr == nil {
				return nil, errors.New("no feegrant available")
			}

			cc = cc.WithFeeGranterAddress(sdk.MustAccAddressFromBech32(fr.Allowance.Granter))
		}
	}

	txf, err := tx.NewFactoryCLI(cc, &pflag.FlagSet{})
	if err != nil {
		return nil, errors.Wrap(err, "error creating tx factory")
	}

	txf = txf.WithGasAdjustment(GAS_ADJUSTMENT)

	gc, err := glib.NewClient(ctx, cc, txf)
	if err != nil {
		return nil, errors.Wrap(err, "error creating cosmos client")
	}

	o := OSKeyring{
		kb:      newKeyringBackend(key, backend, gc),
		secType: KEYRING_BACKEND,
	}

	o.address = o.kb.CC.Address().String()

	return o, nil
}

func (o OSKeyring) SignData(data []byte) (string, error) {
	var privKey offchaintypes.SignatureProvider

	encConf := gitopia.MakeEncodingConfig()
	offchaintypes.RegisterInterfaces(encConf.InterfaceRegistry)
	offchaintypes.RegisterLegacyAminoCodec(encConf.Amino)

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	k, err := sdkkeyring.New(AppName, o.kb.backend, "", os.Stdin, codec.NewProtoCodec(registry))
	if err != nil {
		return "", err
	}
	record, err := k.Key(o.kb.key)
	if err != nil {
		return "", err
	}
	if record.GetType() == sdkkeyring.TypeLocal {
		rl := record.GetLocal()
		if rl.PrivKey == nil {
			return "", errors.New("private key is not available")
		}

		var ok bool
		privKey, ok = rl.PrivKey.GetCachedValue().(cryptotypes.PrivKey)
		if !ok {
			return "", errors.New("unable to cast any to cryptotypes.PrivKey")
		}

	} else {
		return "", fmt.Errorf("fatal: unsupported keyring backend: %v", record.GetType())
	}

	signer := offchaintypes.NewSigner(encConf.TxConfig, privKey)
	accAddress, err := sdk.AccAddressFromBech32(o.Address())
	signData := offchaintypes.NewMsgSignData(accAddress, data)

	tx, err := signer.Sign([]sdk.Msg{signData})
	if err != nil {
		return "", errors.Wrap(err, "error signing data")
	}

	txBz, err := encConf.TxConfig.TxJSONEncoder()(tx)
	if err != nil {
		return "", errors.Wrap(err, "error encoding tx")
	}

	return string(txBz), nil
}

func (o OSKeyring) SignAndBroadcast(grpcConn *grpc.ClientConn, msgs []sdk.Msg) error {
	return o.kb.CC.BroadcastTxAndWait(context.Background(), msgs...)
}

func (o OSKeyring) Address() string {
	return o.address
}

func (o OSKeyring) Type() secretType {
	return o.secType
}

func (o OSKeyring) KeyName() string {
	return o.kb.key
}
