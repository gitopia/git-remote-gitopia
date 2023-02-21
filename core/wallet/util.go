package wallet

import (
	"context"
	"math"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/gitopia/git-remote-gitopia/config"
	"google.golang.org/grpc"
)

const (
	GAS_ADJUSTMENT = 1.5
)

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

func calculateFee(gas uint64) (sdk.Coins, error) {
	gasPrice, err := sdk.ParseDecCoin(config.GasPrices)
	if err != nil {
		return nil, err
	}
	fee := float64(gas) * float64(gasPrice.Amount.MustFloat64())
	fee = math.Ceil(fee)

	return sdk.NewCoins(sdk.NewCoin(gasPrice.Denom, sdk.NewInt(int64(fee)))), nil
}
