package main

import (
	"context"
	"fmt"
	"os"

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

func signAndBroadcastTx(cc *grpc.ClientConn, sender string, chainId string, privKey cryptotypes.PrivKey, msg sdk.Msg) error {
	accountQueryClient := authtype.NewQueryClient(cc)
	txClient := tx.NewServiceClient(cc)

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
	txBuilder.SetMsgs(msg)
	txBuilder.SetGasLimit(200000)

	res, err := accountQueryClient.Account(context.Background(),
		&authtype.QueryAccountRequest{
			Address: sender,
		},
	)
	var acc authtype.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return err
	}

	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  txCfg.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: acc.GetSequence(),
	}
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return err
	}

	signerData := xauthsigning.SignerData{
		ChainID:       chainId,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}

	sigV2, err = clientTx.SignWithPrivKey(txCfg.SignModeHandler().DefaultMode(), signerData,
		txBuilder, privKey, txCfg, acc.GetSequence())
	if err != nil {
		return err
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return err
	}

	err = txBuilder.GetTx().ValidateBasic()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: tx validation failed: %v", err.Error())
	}

	var txBytes []byte
	txBytes, err = txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return err
	}

	var grpcRes *tx.BroadcastTxResponse
	grpcRes, err = txClient.BroadcastTx(
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
