package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	clientTx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cosmoscryptoed "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cosmoscryptosecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtype "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	"github.com/gitopia/git-remote-gitopia/config"
	gitopia "github.com/gitopia/gitopia/v6/app"
	offchaintypes "github.com/gitopia/gitopia/v6/x/offchain/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type Account struct {
	Address       string `json:"address"`
	PathIncrement int    `json:"pathIncrement"`
}

type GitopiaWalletFile struct {
	Name          string    `json:"name"`
	Mnemonic      string    `json:"mnemonic"`
	HDpath        string    `json:"HDpath"`
	Password      string    `json:"password"`
	Prefix        string    `json:"prefix"`
	PathIncrement int       `json:"pathIncrement"`
	Accounts      []Account `json:"accounts"`
}

type GitopiaWallet struct {
	walletFile     GitopiaWalletFile
	privateKey     cryptotypes.PrivKey
	address        string
	secType        secretType
	feeGranterAddr string
}

func InitGitopiaWallet(bankClient banktypes.QueryClient, feegrantClient feegrant.QueryClient) (Wallet, error) {
	var buffer []byte
	var err error
	gw := GitopiaWallet{}

	// Read wallet
	isGitHubAction := os.Getenv("GITHUB_ACTIONS")
	if isGitHubAction == "true" {
		// Read from GitHub secret
		buffer = []byte(os.Getenv("GITOPIA_WALLET"))
		gw.secType = GITHUB_SEC
	} else if len(os.Getenv("GITOPIA_WALLET")) != 0 {
		gitopiaWalletPath := os.Getenv("GITOPIA_WALLET")
		buffer, err = os.ReadFile(gitopiaWalletPath)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read gitopia wallet")
		}
		gw.secType = ENV_VAR
	} else {
		return nil, errors.New("GITOPIA_WALLET environment variable is not set")
	}

	err = json.Unmarshal(buffer, &gw.walletFile)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding wallet file")
	}

	gw.privateKey, err = gw.privKey()
	if err != nil {
		return nil, err
	}
	gw.address = sdk.AccAddress(gw.privateKey.PubKey().Address()).String()

	if bankClient != nil && feegrantClient != nil {
		b, err := bankClient.Balance(context.Background(), &banktypes.QueryBalanceRequest{
			Address: gw.address,
			Denom:   config.Denom,
		})
		if err != nil {
			return nil, errors.New("error querying for balance")
		}

		// Use fee grant only when balance is zero
		if b.Balance.Amount.IsZero() {
			fr, _ := feegrantClient.Allowance(context.Background(), &feegrant.QueryAllowanceRequest{
				Granter: config.FeeGranterAddr,
				Grantee: gw.address,
			})
			if fr == nil {
				return nil, errors.New("no feegrant available")
			}

			gw.feeGranterAddr = fr.Allowance.Granter
		}
	}

	return gw, nil
}

func (gw GitopiaWallet) SignData(data []byte) (string, error) {
	encConf := gitopia.MakeEncodingConfig()
	offchaintypes.RegisterInterfaces(encConf.InterfaceRegistry)
	offchaintypes.RegisterLegacyAminoCodec(encConf.Amino)

	signer := offchaintypes.NewSigner(encConf.TxConfig, gw.privateKey)
	accAddress, err := sdk.AccAddressFromBech32(gw.Address())
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

func (gw GitopiaWallet) SignAndBroadcast(grpcConn *grpc.ClientConn, msgs []sdk.Msg) error {
	txClient := tx.NewServiceClient(grpcConn)
	accountQueryClient := authtype.NewQueryClient(grpcConn)
	interfaceRegistry := types.NewInterfaceRegistry()
	interfaceRegistry.RegisterInterface(
		"cosmos.auth.v1beta1.AccountI",
		(*authtype.AccountI)(nil),
		&authtype.BaseAccount{},
		&authtype.ModuleAccount{},
	)
	interfaceRegistry.RegisterInterface("cosmos.crypto.PubKey", (*cryptotypes.PubKey)(nil))
	interfaceRegistry.RegisterImplementations((*cryptotypes.PubKey)(nil), &cosmoscryptosecp.PubKey{})
	interfaceRegistry.RegisterImplementations((*cryptotypes.PubKey)(nil), &cosmoscryptoed.PubKey{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txCfg := authtx.NewTxConfig(marshaler, authtx.DefaultSignModes)

	txBuilder := txCfg.NewTxBuilder()
	err := txBuilder.SetMsgs(msgs...)
	if err != nil {
		return err
	}

	res, err := accountQueryClient.Account(context.Background(),
		&authtype.QueryAccountRequest{
			Address: gw.Address(),
		},
	)
	if err != nil {
		return err
	}
	var acc authtype.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return err
	}

	signMode := txCfg.SignModeHandler().DefaultMode()
	sigV2 := signing.SignatureV2{
		PubKey: gw.privateKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signMode,
			Signature: nil,
		},
		Sequence: acc.GetSequence(),
	}
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return err
	}

	gas, err := calculateGas(grpcConn, txClient, txCfg, txBuilder)
	if err != nil {
		return err
	}
	txBuilder.SetGasLimit(gas)

	fee, err := calculateFee(gas)
	if err != nil {
		return err
	}
	txBuilder.SetFeeAmount(fee)

	if gw.feeGranterAddr != "" {
		feeGranterAddr, err := sdk.AccAddressFromBech32(gw.feeGranterAddr)
		if err != nil {
			return err
		}
		feePayerAddr, err := sdk.AccAddressFromBech32(gw.Address())
		if err != nil {
			return err
		}

		txBuilder.SetFeeGranter(feeGranterAddr)
		txBuilder.SetFeePayer(feePayerAddr)
	}

	// Get chain id for signing transaction
	serviceClient := tmservice.NewServiceClient(grpcConn)
	nodeInfoRes, err := serviceClient.GetNodeInfo(context.Background(), &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return err
	}
	chainId := nodeInfoRes.DefaultNodeInfo.Network

	signerData := xauthsigning.SignerData{
		ChainID:       chainId,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}

	sigV2, err = clientTx.SignWithPrivKey(signMode, signerData,
		txBuilder, gw.privateKey, txCfg, acc.GetSequence())
	if err != nil {
		return err
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return err
	}

	err = txBuilder.GetTx().ValidateBasic()
	if err != nil {
		return errors.Wrap(err, "tx validation failed")
	}

	txBytes, err := txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return err
	}

	grpcRes, err := txClient.BroadcastTx(
		context.Background(),
		&tx.BroadcastTxRequest{
			Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
			TxBytes: txBytes,
		},
	)
	if err != nil {
		return err
	}

	if grpcRes.TxResponse.Code != 0 {
		return fmt.Errorf("fatal: failed to broadcast transaction: %v", grpcRes.TxResponse)
	}

	return nil
}

func (gw GitopiaWallet) Address() string {
	return gw.address
}

func (gw GitopiaWallet) Type() secretType {
	return gw.secType
}

func (gw GitopiaWallet) privKey() (cryptotypes.PrivKey, error) {
	// Generate private key
	hdPath := gw.walletFile.HDpath + strconv.Itoa(gw.walletFile.PathIncrement)
	derivedPriv, err := hd.Secp256k1.Derive()(gw.walletFile.Mnemonic, "", hdPath)
	if err != nil {
		return nil, err
	}

	privKey := hd.Secp256k1.Generate()(derivedPriv)
	return privKey, nil
}
