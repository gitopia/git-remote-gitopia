package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	clientTx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authTx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authType "github.com/cosmos/cosmos-sdk/x/auth/types"
	core "github.com/gitopia/git-remote-gitopia/core"
	gitopiaTypes "github.com/gitopia/gitopia/x/gitopia/types"

	// "github.com/gitopia/gitopia/x/gitopia/utils"
	"github.com/go-git/go-git/v5"
	goGitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"google.golang.org/grpc"
)

const (
	chainID              = "internal-2"
	AccountAddressPrefix = "gitopia"
	address              = "34.87.152.178:9090"
)

var (
	AccountPubKeyPrefix = AccountAddressPrefix + "pub"
)

type Account struct {
	Address       string `json:"address"`
	PathIncrement int    `json:"pathIncrement"`
}

type GitopiaWallet struct {
	Name          string    `json:"name"`
	Mnemonic      string    `json:"mnemonic"`
	HDpath        string    `json:"HDpath"`
	Password      string    `json:"password"`
	Prefix        string    `json:"prefix"`
	PathIncrement int       `json:"pathIncrement"`
	Accounts      []Account `json:"accounts"`
}

type Owner struct {
	Type string
	ID   string
}

type GitopiaHandler struct {
	queryClient        gitopiaTypes.QueryClient
	accountQueryClient authType.QueryClient
	txClient           tx.ServiceClient

	remoteUserId          string
	remoteRepositoryName  string
	remoteRepositoryId    uint64
	remoteDefaultBranch   string
	remoteRepositoryOwner Owner

	didPush bool
}

func (h *GitopiaHandler) Initialize(remote *core.Remote) error {
	grpcConn, err := grpc.Dial(address,
		grpc.WithInsecure(),
	)
	if err != nil {
		return err
	}
	// defer grpcConn.Close()

	h.queryClient = gitopiaTypes.NewQueryClient(grpcConn)
	h.accountQueryClient = authType.NewQueryClient(grpcConn)
	h.txClient = tx.NewServiceClient(grpcConn)

	// Get RepositoryId
	params := &gitopiaTypes.QueryGetUserRepositoryRequest{
		UserId:         h.remoteUserId,
		RepositoryName: h.remoteRepositoryName,
	}

	res, err := h.queryClient.UserRepository(context.Background(), params)
	if err != nil {
		return fmt.Errorf("fatal: repository 'gitopia://%s/%s' not found", h.remoteUserId, h.remoteRepositoryName)
	}

	h.remoteRepositoryId = res.Repository.Id
	h.remoteDefaultBranch = res.Repository.DefaultBranch

	if err = json.Unmarshal([]byte(res.Repository.Owner), &h.remoteRepositoryOwner); err != nil {
		return fmt.Errorf("fatal: repository owner is malformed")
	}

	remoteURL := fmt.Sprintf("http://34.126.69.254:5000/%v.git", h.remoteRepositoryId)

	remoteConfig := &goGitConfig.RemoteConfig{
		Name: "gitopia-objects-store",
		URLs: []string{remoteURL},
	}

	_, err = remote.Repo.CreateRemote(remoteConfig)
	if err != nil {
		return err
	}

	return nil
}

func (h *GitopiaHandler) Finish(remote *core.Remote) error {
	if h.didPush {
		remote.Logger.Printf("Pushed to Gitopia\n")
	}

	remote.Repo.DeleteRemote("gitopia-objects-store")

	return nil
}

func (h *GitopiaHandler) List(remote *core.Remote, forPush bool) ([]string, error) {
	out := make([]string, 0)

	params := &gitopiaTypes.QueryGetAllBranchRequest{
		Id: h.remoteRepositoryId,
	}

	res, err := h.queryClient.BranchAll(context.Background(), params)
	if err != nil {
		return out, err
	}

	for refName, hash := range res.Branches {
		out = append(out, fmt.Sprintf("%s %s", hash, refName))
	}

	out = append(out, fmt.Sprintf("@refs/heads/%s HEAD", h.remoteDefaultBranch))

	return out, nil
}

func (h *GitopiaHandler) Push(remote *core.Remote, local string, remoteRef string) (string, error) {
	fmt.Fprintf(os.Stderr, "local: %s remote: %s\n", local, remoteRef)
	h.didPush = true

	gitopiaWalletPath := os.Getenv("GITOPIA_WALLET")
	if gitopiaWalletPath == "" {
		return "", fmt.Errorf("fatal: GITOPIA_WALLET environment variable is not set")
	}

	buffer, err := os.ReadFile(gitopiaWalletPath)
	if err != nil {
		return "", fmt.Errorf("fatal: error reading gitopia wallet")
	}

	var gitopiaWallet GitopiaWallet
	err = json.Unmarshal(buffer, &gitopiaWallet)
	if err != nil {
		return "", fmt.Errorf("fatal: error decoding wallet file")
	}

	// Check push access
	derivedPriv, err := hd.Secp256k1.Derive()(gitopiaWallet.Mnemonic, "", gitopiaWallet.HDpath)
	if err != nil {
		return "", err
	}

	privKey := hd.Secp256k1.Generate()(derivedPriv)

	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(AccountAddressPrefix, AccountPubKeyPrefix)
	config.Seal()

	address := sdk.AccAddress(privKey.PubKey().Address())

	pushOptions := &git.PushOptions{
		RemoteName: "gitopia-objects-store",
		RefSpecs:   []goGitConfig.RefSpec{goGitConfig.RefSpec(fmt.Sprintf("%s:%s", local, remoteRef))},
	}

	err = remote.Repo.Push(pushOptions)
	// if err != nil {
	// 	return "", fmt.Errorf("fatal: error pushing the git objects")
	// }

	// Update ref on gitopia
	interfaceRegistry := types.NewInterfaceRegistry()
	interfaceRegistry.RegisterInterface(
		"cosmos.auth.v1beta1.AccountI",
		(*authType.AccountI)(nil),
		&authType.BaseAccount{},
		&authType.ModuleAccount{},
	)
	interfaceRegistry.RegisterImplementations(cryptotypes.PubKey, secp256k1.PubKey)
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txCfg := authTx.NewTxConfig(marshaler, authTx.DefaultSignModes)

	txBuilder := txCfg.NewTxBuilder()

	localCommitSHA, err := remote.Repo.ResolveRevision(plumbing.Revision(local))
	if err != nil {
		return "", fmt.Errorf("fatal: local branch %s doesn't exist", local)
	}

	msg := gitopiaTypes.NewMsgCreateBranch(address.String(), h.remoteRepositoryId, remoteRef, localCommitSHA.String())
	txBuilder.SetMsgs(msg)

	// txBuilder.SetGasLimit()

	res, err := h.accountQueryClient.Account(context.Background(),
		&authType.QueryAccountRequest{
			Address: address.String(),
		},
	)
	var acc authType.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return "", err
	}

	// sigV2 := signing.SignatureV2{
	// 	PubKey: privKey.PubKey(),
	// 	Data: &signing.SingleSignatureData{
	// 		SignMode: txCfg.SignModeHandler().DefaultMode(),
	// 		Signature: nil,
	// 	},
	// 	Sequence: acc.GetSequence(),
	// }
	// err = txBuilder.SetSignatures(sigV2)
	// if err != nil {
	// 	return "", err
	// }

	signerData := xauthsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}
	var sigV2 signing.SignatureV2
	sigV2, err = clientTx.SignWithPrivKey(txCfg.SignModeHandler().DefaultMode(), signerData,
		txBuilder, privKey, txCfg, acc.GetSequence())
	if err != nil {
		return "", err
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return "", err
	}

	var txBytes []byte
	txBytes, err = txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return "", err
	}

	var grpcRes *tx.BroadcastTxResponse
	grpcRes, err = h.txClient.BroadcastTx(
		context.Background(),
		&tx.BroadcastTxRequest{
			Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
			TxBytes: txBytes,
		},
	)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "transaction response code: %v\n", grpcRes.TxResponse.Code)

	return local, nil
}

func (h *GitopiaHandler) checkPushAccess() (bool, error) {
	// TODO

	return true, nil
}

func (h *GitopiaHandler) getRef(name string) (string, error) {
	// TODO

	return "", nil
}
