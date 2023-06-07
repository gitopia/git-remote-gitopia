package wallet

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/gitopia/git-remote-gitopia/config"
	glib "github.com/gitopia/gitopia-go"
	"github.com/gitopia/gitopia-go/logger"
	gitopia "github.com/gitopia/gitopia/v2/app"
	offchaintypes "github.com/gitopia/gitopia/v2/x/offchain/types"
	goGitConfig "github.com/go-git/go-git/v5/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/cosmos/cosmos-sdk/std"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	gtypes "github.com/gitopia/gitopia/v2/x/gitopia/types"
	otypes "github.com/gitopia/gitopia/v2/x/offchain/types"
	rtypes "github.com/gitopia/gitopia/v2/x/rewards/types"

	"github.com/pkg/errors"
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

func InitOSKeyringWallet() (Wallet, error) {
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
	glib.WithGitopiaAddr(config.GRPCHost)
	glib.WithGasPrices(config.GasPrices)
	cc, err := NewContext(key)
	if err != nil {
		return nil, errors.Wrap(err, "error creating cosmos client context")
	}
	txf := tx.NewFactoryCLI(cc, &pflag.FlagSet{}).WithGasAdjustment(GAS_ADJUSTMENT)
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

func NewContext(from string) (client.Context, error) {
	version.Name = AppName
	clientCtx := client.Context{}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	cryptocodec.RegisterInterfaces(interfaceRegistry)
	authtypes.RegisterInterfaces(interfaceRegistry)
	gtypes.RegisterInterfaces(interfaceRegistry)
	rtypes.RegisterInterfaces(interfaceRegistry)
	otypes.RegisterInterfaces(interfaceRegistry)

	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txCfg := authtx.NewTxConfig(marshaler, authtx.DefaultSignModes)
	clientCtx = clientCtx.
		WithCodec(marshaler).
		WithInterfaceRegistry(interfaceRegistry).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithTxConfig(txCfg).
		WithInput(os.Stdin)

	clientCtx = clientCtx.WithChainID(config.ChainId)
	clientCtx = clientCtx.WithNodeURI(config.TmAddr)
	c, err := client.NewClientFromNode(clientCtx.NodeURI)
	if err != nil {
		return clientCtx, errors.Wrap(err, "error creatig tm client")
	}
	clientCtx = clientCtx.WithClient(c)
	clientCtx = clientCtx.WithBroadcastMode(flags.BroadcastSync)
	clientCtx = clientCtx.WithSkipConfirmation(true)

	kr, err := client.NewKeyringFromBackend(clientCtx, keyring.BackendOS)
	if err != nil {
		return clientCtx, errors.Wrap(err, "error creating keyring backend")
	}
	clientCtx = clientCtx.WithKeyring(kr)

	fromAddr, fromName, _, err := client.GetFromFields(clientCtx, kr, from)
	if err != nil {
		return clientCtx, errors.Wrap(err, "error parsing from Addr")
	}

	clientCtx = clientCtx.WithFrom(from).WithFromAddress(fromAddr).WithFromName(fromName)

	feeGranterAddr := sdk.MustAccAddressFromBech32(config.FeeGranterAddr)
	clientCtx = clientCtx.WithFeeGranterAddress(feeGranterAddr)
	return clientCtx, nil
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
