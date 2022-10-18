package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	clientTx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	cosmoscryptoed "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cosmoscryptosecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtype "github.com/cosmos/cosmos-sdk/x/auth/types"

	"google.golang.org/grpc"
)

const (
	GAS_ADJUSTMENT = 1.2
)

func signWithWallet(cc *grpc.ClientConn, sender string, chainId string, privKey cryptotypes.PrivKey, msg []sdk.Msg, txClient tx.ServiceClient) ([]byte, error) {
	accountQueryClient := authtype.NewQueryClient(cc)
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
	err := txBuilder.SetMsgs(msg...)
	if err != nil {
		return nil, err
	}
	txBuilder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin("utlore", sdk.NewInt(500))))

	res, err := accountQueryClient.Account(context.Background(),
		&authtype.QueryAccountRequest{
			Address: sender,
		},
	)
	if err != nil {
		return nil, err
	}
	var acc authtype.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return nil, err
	}

	signMode := txCfg.SignModeHandler().DefaultMode()
	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signMode,
			Signature: nil,
		},
		Sequence: acc.GetSequence(),
	}
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	gas, err := calculateGas(cc, txClient, txCfg, txBuilder)
	if err != nil {
		return nil, err
	}
	txBuilder.SetGasLimit(gas)

	signerData := xauthsigning.SignerData{
		ChainID:       chainId,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}

	sigV2, err = clientTx.SignWithPrivKey(signMode, signerData,
		txBuilder, privKey, txCfg, acc.GetSequence())
	if err != nil {
		return nil, err
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	err = txBuilder.GetTx().ValidateBasic()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: tx validation failed: %v", err.Error())
	}

	txBytes, err := txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	return txBytes, nil
}

func signWithLedger(cc *grpc.ClientConn, sender string, chainId string, ledgerPrivKey cryptotypes.LedgerPrivKey, msg []sdk.Msg, txClient tx.ServiceClient) ([]byte, error) {
	accountQueryClient := authtype.NewQueryClient(cc)
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
	err := txBuilder.SetMsgs(msg...)
	if err != nil {
		return nil, err
	}
	txBuilder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin("utlore", sdk.NewInt(500))))

	res, err := accountQueryClient.Account(context.Background(),
		&authtype.QueryAccountRequest{
			Address: sender,
		},
	)
	if err != nil {
		return nil, err
	}
	var acc authtype.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return nil, err
	}

	signMode := signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
	sigV2 := signing.SignatureV2{
		PubKey: ledgerPrivKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signMode,
			Signature: nil,
		},
		Sequence: acc.GetSequence(),
	}
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	gas, err := calculateGas(cc, txClient, txCfg, txBuilder)
	if err != nil {
		return nil, err
	}
	txBuilder.SetGasLimit(gas)

	signerData := xauthsigning.SignerData{
		Address:       ledgerPrivKey.PubKey().String(),
		ChainID:       chainId,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}

	bytesToSign, err := txCfg.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	// TODO: Gives error in case of large msg
	// JSON. Too many tokens
	sigBytes, err := ledgerPrivKey.Sign(bytesToSign)
	if err != nil {
		return nil, err
	}

	// Construct the SignatureV2 struct
	sigData := signing.SingleSignatureData{
		SignMode:  signMode,
		Signature: sigBytes,
	}
	sigV2 = signing.SignatureV2{
		PubKey:   ledgerPrivKey.PubKey(),
		Data:     &sigData,
		Sequence: acc.GetSequence(),
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	err = txBuilder.GetTx().ValidateBasic()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: tx validation failed: %v", err.Error())
	}

	txBytes, err := txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	return txBytes, nil
}

func signAndBroadcastTx(cc *grpc.ClientConn, sender string, chainId string, privKey cryptotypes.PrivKey,
	ledgerPrivKey cryptotypes.LedgerPrivKey, msg []sdk.Msg, useLedger bool) error {
	txClient := tx.NewServiceClient(cc)

	var txBytes []byte
	var err error
	if useLedger {
		txBytes, err = signWithLedger(cc, sender, chainId, ledgerPrivKey, msg, txClient)
		if err != nil {
			return err
		}
	} else {
		txBytes, err = signWithWallet(cc, sender, chainId, privKey, msg, txClient)
		if err != nil {
			return err
		}
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

func calculateGas(cc *grpc.ClientConn, txClient tx.ServiceClient, txCfg client.TxConfig, txBuilder client.TxBuilder) (uint64, error) {
	txBytes, err := txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return 0, err
	}

	simRes, err := txClient.Simulate(context.Background(), &tx.SimulateRequest{
		TxBytes: txBytes,
	})
	if err != nil {
		return 0, err
	}

	gas := uint64(GAS_ADJUSTMENT * float64(simRes.GasInfo.GasUsed))

	return gas, nil
}
