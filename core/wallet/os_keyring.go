package wallet

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	"github.com/gitopia/git-remote-gitopia/config"
	gitopia "github.com/gitopia/gitopia/v2/app"
	offchaintypes "github.com/gitopia/gitopia/v2/x/offchain/types"
	goGitConfig "github.com/go-git/go-git/v5/config"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/ignite/pkg/cosmosclient"
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
	CC      cosmosclient.Client
}

func newKeyringBackend(k string, b string, c cosmosclient.Client) keyringBackend {
	c.TxFactory = c.TxFactory.WithGasPrices(config.GasPrices)
	return keyringBackend{
		key:     k,
		backend: b,
		CC:      c,
	}
}

func (k keyringBackend) address() (string, error) {
	address, err := k.CC.Address(k.key)
	return address, err
}

type OSKeyring struct {
	kb      keyringBackend
	address string
	secType secretType
}

func InitOSKeyringWallet() (Wallet, error) {
	var key string
	var backend string
	var cc cosmosclient.Client

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

	cc, err = cosmosclient.New(context.Background(),
		cosmosclient.WithNodeAddress(config.TmAddr),
		// same service name used in both helper and keys management app
		cosmosclient.WithKeyringServiceName(AppName),                           // not suported on macos
		cosmosclient.WithKeyringBackend(cosmosaccount.KeyringBackend(backend)), // not all backends supported by cosmos are supported by cosmos client
		cosmosclient.WithAddressPrefix(AccountAddressPrefix),
	)
	if err != nil {
		return nil, errors.Wrap(err, "error creating cosmos client")
	}

	o := OSKeyring{
		kb:      newKeyringBackend(key, backend, cc),
		secType: KEYRING_BACKEND,
	}

	o.address, err = o.kb.address()
	if err != nil {
		return nil, err
	}

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
	account, err := o.kb.CC.Account(o.kb.key)
	if err != nil {
		return err
	}

	// check fee grant exists
	fqc := feegrant.NewQueryClient(grpcConn)
	fr, _ := fqc.Allowance(context.Background(), &feegrant.QueryAllowanceRequest{
		Granter: config.FeeGranterAddr,
		Grantee: o.Address(),
	})

	if fr != nil {
		feeGranterAddr, err := sdk.AccAddressFromBech32(config.FeeGranterAddr)
		if err != nil {
			return err
		}
		feePayerAddr, err := sdk.AccAddressFromBech32(o.Address())
		if err != nil {
			return err
		}

		// Configure feegranter in cosmosclient
		cosmosclient.WithFeeGranterAddress(feeGranterAddr)(&o.kb.CC)
		
		o.kb.CC.TxFactory = o.kb.CC.TxFactory.WithFeePayer(feePayerAddr)
	}

	txResp, err := o.kb.CC.BroadcastTx(account, msgs...)
	if err != nil {
		return err
	}
	if txResp.TxResponse.Code != 0 {
		return errors.Wrap(err, "error broadcasting transaction")
	}

	return nil
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
