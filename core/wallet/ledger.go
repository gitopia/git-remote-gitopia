package wallet

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cosmoscryptoed "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cosmoscryptosecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/ledger"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtype "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	"github.com/gitopia/git-remote-gitopia/config"
	gitopia "github.com/gitopia/gitopia/v5/app"
	offchaintypes "github.com/gitopia/gitopia/v5/x/offchain/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type Ledger struct {
	privateKey     cryptotypes.LedgerPrivKey
	address        string
	secType        secretType
	feeGranterAddr string
}

func InitLedgerWallet(bankClient banktypes.QueryClient, feegrantClient feegrant.QueryClient) (Wallet, error) {
	ledgerPrivKey, err := ledger.NewPrivKeySecp256k1Unsafe(hd.BIP44Params{
		Purpose:      44,
		CoinType:     118,
		Account:      0,
		Change:       false,
		AddressIndex: 0,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error generating ledger key")
	}

	addr := sdk.AccAddress(ledgerPrivKey.PubKey().Address()).String()

	feeGranter := ""
	if bankClient != nil && feegrantClient != nil {
		b, err := bankClient.Balance(context.Background(), &banktypes.QueryBalanceRequest{
			Address: addr,
			Denom:   config.Denom,
		})
		if err != nil {
			return nil, errors.New("error querying for balance")
		}

		// Use fee grant only when balance is zero
		if b.Balance.Amount.IsZero() {
			fr, _ := feegrantClient.Allowance(context.Background(), &feegrant.QueryAllowanceRequest{
				Granter: config.FeeGranterAddr,
				Grantee: addr,
			})
			if fr == nil {
				return nil, errors.New("no feegrant available")
			}

			feeGranter = fr.Allowance.Granter
		}
	}

	return Ledger{
		privateKey:     ledgerPrivKey,
		secType:        LEDGER,
		address:        addr,
		feeGranterAddr: feeGranter,
	}, nil
}

func (l Ledger) SignData(data []byte) (string, error) {
	encConf := gitopia.MakeEncodingConfig()
	offchaintypes.RegisterInterfaces(encConf.InterfaceRegistry)
	offchaintypes.RegisterLegacyAminoCodec(encConf.Amino)

	// Set legacy amino json as sign mode in case of ledger
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(encConf.InterfaceRegistry)
	gitopia.ModuleBasics.RegisterInterfaces(encConf.InterfaceRegistry)
	offchaintypes.RegisterInterfaces(interfaceRegistry)
	cryptocodec.RegisterInterfaces(interfaceRegistry)
	codec := codec.NewProtoCodec(interfaceRegistry)
	txCfg := authtx.NewTxConfig(codec, []signing.SignMode{signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON})
	encConf.TxConfig = txCfg

	signer := offchaintypes.NewSigner(encConf.TxConfig, l.privateKey)
	accAddress, err := sdk.AccAddressFromBech32(l.Address())
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

func (l Ledger) SignAndBroadcast(grpcConn *grpc.ClientConn, msgs []sdk.Msg) error {
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
			Address: l.Address(),
		},
	)
	if err != nil {
		return err
	}
	var acc authtype.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return err
	}

	signMode := signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
	sigV2 := signing.SignatureV2{
		PubKey: l.privateKey.PubKey(),
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

	if l.feeGranterAddr != "" {
		feeGranterAddr, err := sdk.AccAddressFromBech32(l.feeGranterAddr)
		if err != nil {
			return err
		}
		feePayerAddr, err := sdk.AccAddressFromBech32(l.Address())
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
		Address:       l.privateKey.PubKey().String(),
		ChainID:       chainId,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}

	bytesToSign, err := txCfg.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return err
	}

	// TODO: Gives error in case of large msg
	// JSON. Too many tokens
	sigBytes, err := l.privateKey.Sign(bytesToSign)
	if err != nil {
		return err
	}

	// Construct the SignatureV2 struct
	sigData := signing.SingleSignatureData{
		SignMode:  signMode,
		Signature: sigBytes,
	}
	sigV2 = signing.SignatureV2{
		PubKey:   l.privateKey.PubKey(),
		Data:     &sigData,
		Sequence: acc.GetSequence(),
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

func (l Ledger) Type() secretType {
	return l.secType
}

func (l Ledger) Address() string {
	return l.address
}
