package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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
	apiURL               = "34.87.152.178:9090"
	objectsURL           = "http://34.126.69.254:5000"
	saveToArweaveURL     = "http://34.126.69.254:5000/save"
	branchPrefix         = "refs/heads/"
	tagPrefix            = "refs/tags/"
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

type SaveToArweavePostBody struct {
	RepositoryID     uint64 `json:"repository_id"`
	LocalObjectHash  string `json:"local_object_hash"`
	RemoteObjectHash string `json:"remote_object_hash"`
}

type GitopiaHandler struct {
	queryClient        gitopiaTypes.QueryClient
	accountQueryClient authType.QueryClient
	txClient           tx.ServiceClient

	remoteUserId          string
	remoteRepositoryName  string
	remoteRepositoryId    uint64
	remoteDefaultBranch   string
	remoteRepositoryOwner gitopiaTypes.RepositoryOwner

	didPush bool
}

func (h *GitopiaHandler) Initialize(remote *core.Remote) error {
	grpcConn, err := grpc.Dial(apiURL,
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
	res, err := h.queryClient.UserRepository(context.Background(), &gitopiaTypes.QueryGetUserRepositoryRequest{
		UserId:         h.remoteUserId,
		RepositoryName: h.remoteRepositoryName,
	})
	if err != nil {
		return fmt.Errorf("fatal: repository 'gitopia://%s/%s' not found. Please create it from the gitopia webapp", h.remoteUserId, h.remoteRepositoryName)
	}

	h.remoteRepositoryId = res.Repository.Id
	h.remoteDefaultBranch = res.Repository.DefaultBranch
	h.remoteRepositoryOwner = *res.Repository.Owner

	return nil
}

func (h *GitopiaHandler) List(remote *core.Remote, forPush bool) ([]string, error) {
	out := make([]string, 0)

	branchAllRes, err := h.queryClient.BranchAll(context.Background(), &gitopiaTypes.QueryGetAllBranchRequest{
		RepositoryId: h.remoteRepositoryId,
	})
	if err != nil {
		return out, err
	}
	for _, branch := range branchAllRes.Branches {
		out = append(out, fmt.Sprintf("%s %s%s", branch.Sha, branchPrefix, branch.Name))
	}

	tagAllRes, err := h.queryClient.TagAll(context.Background(), &gitopiaTypes.QueryGetAllTagRequest{
		RepositoryId: h.remoteRepositoryId,
	})
	if err != nil {
		return out, err
	}
	for _, tag := range tagAllRes.Tags {
		out = append(out, fmt.Sprintf("%s %s%s", tag.Sha, tagPrefix, tag.Name))
	}

	out = append(out, fmt.Sprintf("@refs/heads/%s HEAD", h.remoteDefaultBranch))

	return out, nil
}

func (h *GitopiaHandler) Fetch(remote *core.Remote, sha, ref string) error {
	remoteURL := fmt.Sprintf("%v/%v.git", objectsURL, h.remoteRepositoryId)
	remoteConfig := &goGitConfig.RemoteConfig{
		Name: "gitopia-objects-store",
		URLs: []string{remoteURL},
	}

	_, err := remote.Repo.CreateRemote(remoteConfig)
	if err != nil {
		return err
	}
	defer remote.Repo.DeleteRemote("gitopia-objects-store")

	fetchOptions := &git.FetchOptions{
		RemoteName: "gitopia-objects-store",
		Progress:   os.Stdout,
	}

	err = remote.Repo.Fetch(fetchOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fatal: %v", err.Error())
	}

	return nil
}

func (h *GitopiaHandler) Push(remote *core.Remote, local string, remoteRef string) (string, error) {
	h.didPush = true
	remoteURL := fmt.Sprintf("%v/%v.git", objectsURL, h.remoteRepositoryId)
	remoteConfig := &goGitConfig.RemoteConfig{
		Name: "gitopia-objects-store",
		URLs: []string{remoteURL},
	}

	_, err := remote.Repo.CreateRemote(remoteConfig)
	if err != nil {
		return "", err
	}
	defer remote.Repo.DeleteRemote("gitopia-objects-store")

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
		Progress:   os.Stdout,
	}

	err = remote.Repo.Push(pushOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("fatal: error pushing the git objects, %v", err.Error())
	}

	// Update ref on gitopia
	interfaceRegistry := types.NewInterfaceRegistry()
	interfaceRegistry.RegisterInterface(
		"cosmos.auth.v1beta1.AccountI",
		(*authType.AccountI)(nil),
		&authType.BaseAccount{},
		&authType.ModuleAccount{},
	)
	interfaceRegistry.RegisterInterface("cosmos.crypto.PubKey", (*cryptotypes.PubKey)(nil))
	interfaceRegistry.RegisterImplementations((*cryptotypes.PubKey)(nil), &cosmoscryptosecp.PubKey{})
	interfaceRegistry.RegisterImplementations((*cryptotypes.PubKey)(nil), &cosmoscryptoed.PubKey{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txCfg := authTx.NewTxConfig(marshaler, authTx.DefaultSignModes)

	txBuilder := txCfg.NewTxBuilder()

	var localObjectHash string
	remoteObjectHash := "0000000000000000000000000000000000000000"
	var msg sdk.Msg

	if strings.HasPrefix(local, branchPrefix) {
		localCommitHash, err := remote.Repo.ResolveRevision(plumbing.Revision(local))
		if err != nil {
			return "", fmt.Errorf("fatal: local branch %s doesn't exist", local)
		}
		localObjectHash = localCommitHash.String()

		remoteBranchName := strings.TrimPrefix(remoteRef, branchPrefix)
		branchShaResponse, err := h.queryClient.BranchSha(context.Background(), &gitopiaTypes.QueryGetBranchShaRequest{
			RepositoryId: h.remoteRepositoryId,
			BranchName:   remoteBranchName,
		})
		if err == nil {
			remoteObjectHash = branchShaResponse.Sha
		}

		msg = gitopiaTypes.NewMsgCreateBranch(address.String(), h.remoteRepositoryId, remoteBranchName, localObjectHash)
	} else if strings.HasPrefix(local, tagPrefix) {
		localTagName := strings.TrimPrefix(local, tagPrefix)
		ref, err := remote.Repo.Tag(localTagName)
		if err != nil {
			return "", fmt.Errorf("fatal: invalid tag name, %v", localTagName)
		}
		localObjectHash = ref.Hash().String()

		remoteTagName := strings.TrimPrefix(remoteRef, tagPrefix)
		tagShaResponse, err := h.queryClient.TagSha(context.Background(), &gitopiaTypes.QueryGetTagShaRequest{
			RepositoryId: h.remoteRepositoryId,
			TagName:      remoteTagName,
		})
		if err == nil {
			remoteObjectHash = tagShaResponse.Sha
		}

		msg = gitopiaTypes.NewMsgCreateTag(address.String(), h.remoteRepositoryId, remoteTagName, ref.Hash().String())
	} else {
		return "", fmt.Errorf("fatal: not a valid branch/tag, %v", local)
	}

	txBuilder.SetMsgs(msg)
	txBuilder.SetGasLimit(200000)

	res, err := h.accountQueryClient.Account(context.Background(),
		&authType.QueryAccountRequest{
			Address: address.String(),
		},
	)
	var acc authType.AccountI
	if err := interfaceRegistry.UnpackAny(res.Account, &acc); err != nil {
		return "", err
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
		return "", err
	}

	signerData := xauthsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: acc.GetAccountNumber(),
		Sequence:      acc.GetSequence(),
	}

	sigV2, err = clientTx.SignWithPrivKey(txCfg.SignModeHandler().DefaultMode(), signerData,
		txBuilder, privKey, txCfg, acc.GetSequence())
	if err != nil {
		return "", err
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return "", err
	}

	err = txBuilder.GetTx().ValidateBasic()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: tx validation failed: %v", err.Error())
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

	if grpcRes.TxResponse.Code != 0 {
		return "", fmt.Errorf("fatal: failed to broadcast transaction, code: %v", grpcRes.TxResponse.Code)
	}

	// Queue task to upload objects to arweave
	saveToArweavePostBody := SaveToArweavePostBody{
		RepositoryID:     h.remoteRepositoryId,
		LocalObjectHash:  localObjectHash,
		RemoteObjectHash: remoteObjectHash,
	}

	postBody, err := json.Marshal(saveToArweavePostBody)
	if err != nil {
		return "", fmt.Errorf("fatal: failed to serialize post data: %v", err.Error())
	}
	responseBody := bytes.NewBuffer(postBody)
	resp, err := http.Post(saveToArweaveURL, "application/json", responseBody)
	if err != nil {
		return "", fmt.Errorf("fatal: error posting saveToArweave: %v", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fatal: error saving to Arweave")
	}

	return local, nil
}

func (h *GitopiaHandler) checkPushAccess() (bool, error) {
	// TODO

	return true, nil
}
