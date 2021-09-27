package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdk "github.com/cosmos/cosmos-sdk/types"
	core "github.com/gitopia/git-remote-gitopia/core"
	gitopiaTypes "github.com/gitopia/gitopia/x/gitopia/types"
	"github.com/gitopia/gitopia/x/gitopia/utils"

	// "github.com/gitopia/gitopia/x/gitopia/utils"
	"github.com/go-git/go-git/v5"
	goGitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"google.golang.org/grpc"
)

const (
	AccountAddressPrefix = "gitopia"
	apiURL               = "34.126.183.252:9090"
	objectsURL           = "http://34.87.64.22:5000"
	saveToArweaveURL     = "http://34.87.64.22:5000/save"
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
	RemoteRefName    string `json:"remote_ref_name"`
	NewRemoteRefSha  string `json:"new_remote_ref_sha"`
	PrevRemoteRefSha string `json:"prev_remote_ref_sha"`
}

type GitopiaHandler struct {
	grpcConn    *grpc.ClientConn
	queryClient gitopiaTypes.QueryClient

	chainId              string
	remoteUserId         string
	remoteRepositoryName string
	remoteRepository     gitopiaTypes.Repository

	didPush bool
}

func (h *GitopiaHandler) Initialize(remote *core.Remote) error {
	var err error

	h.grpcConn, err = grpc.Dial(apiURL,
		grpc.WithInsecure(),
	)
	if err != nil {
		return err
	}
	// defer grpcConn.Close()

	h.queryClient = gitopiaTypes.NewQueryClient(h.grpcConn)
	serviceClient := tmservice.NewServiceClient(h.grpcConn)

	// Get chain id for signing transaction
	nodeInfoRes, err := serviceClient.GetNodeInfo(context.Background(), &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return err
	}
	h.chainId = nodeInfoRes.DefaultNodeInfo.Network

	// Get RepositoryId
	res, err := h.queryClient.AddressRepository(context.Background(), &gitopiaTypes.QueryGetAddressRepositoryRequest{
		Id:             h.remoteUserId,
		RepositoryName: h.remoteRepositoryName,
	})
	if err != nil {
		return err
	}

	h.remoteRepository = *res.Repository

	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(AccountAddressPrefix, AccountPubKeyPrefix)
	config.Seal()

	return nil
}

func (h *GitopiaHandler) List(remote *core.Remote, forPush bool) ([]string, error) {
	out := make([]string, 0)

	branchAllRes, err := h.queryClient.BranchAll(context.Background(), &gitopiaTypes.QueryGetAllBranchRequest{
		RepositoryId: h.remoteRepository.Id,
	})
	if err != nil {
		return out, err
	}
	for _, branch := range branchAllRes.Branches {
		out = append(out, fmt.Sprintf("%s %s%s", branch.Sha, branchPrefix, branch.Name))
	}

	tagAllRes, err := h.queryClient.TagAll(context.Background(), &gitopiaTypes.QueryGetAllTagRequest{
		RepositoryId: h.remoteRepository.Id,
	})
	if err != nil {
		return out, err
	}
	for _, tag := range tagAllRes.Tags {
		out = append(out, fmt.Sprintf("%s %s%s", tag.Sha, tagPrefix, tag.Name))
	}

	out = append(out, fmt.Sprintf("@refs/heads/%s HEAD", h.remoteRepository.DefaultBranch))

	return out, nil
}

func (h *GitopiaHandler) Fetch(remote *core.Remote, sha, ref string) error {
	remoteURL := fmt.Sprintf("%v/%v.git", objectsURL, h.remoteRepository.Id)
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
	var gitopiaWallet GitopiaWallet
	var buffer []byte
	h.didPush = true

	// Read wallet
	isGitHubAction := os.Getenv("GITHUB_ACTIONS")
	if isGitHubAction == "true" {
		// Read from GitHub secret
		buffer = []byte(os.Getenv("GITOPIA_WALLET"))
	} else {
		gitopiaWalletPath := os.Getenv("GITOPIA_WALLET")
		if gitopiaWalletPath == "" {
			return "", fmt.Errorf("fatal: GITOPIA_WALLET environment variable is not set")
		}

		var err error
		buffer, err = os.ReadFile(gitopiaWalletPath)
		if err != nil {
			return "", fmt.Errorf("fatal: error reading gitopia wallet")
		}

	}

	err := json.Unmarshal(buffer, &gitopiaWallet)
	if err != nil {
		return "", fmt.Errorf("fatal: error decoding wallet file")
	}

	// Generate private key
	hdPath := gitopiaWallet.HDpath + strconv.Itoa(gitopiaWallet.PathIncrement)
	derivedPriv, err := hd.Secp256k1.Derive()(gitopiaWallet.Mnemonic, "", hdPath)
	if err != nil {
		return "", err
	}

	privKey := hd.Secp256k1.Generate()(derivedPriv)
	walletAddress := sdk.AccAddress(privKey.PubKey().Address())

	havePushPermission, err := h.havePushPermission(walletAddress.String())
	if err != nil {
		return "", err
	}
	if !havePushPermission {
		return "", fmt.Errorf("fatal: you don't have write permissions to this repository")
	}

	var msg sdk.Msg

	// Delete branch/tag
	if local == "" {
		if strings.HasPrefix(remoteRef, branchPrefix) {
			remoteBranchName := strings.TrimPrefix(remoteRef, branchPrefix)

			// Check if it's the default branch
			if remoteBranchName == h.remoteRepository.DefaultBranch {
				return "", fmt.Errorf("fatal: cannot delete default branch, %v", remoteBranchName)
			}

			msg = gitopiaTypes.NewMsgDeleteBranch(walletAddress.String(), h.remoteRepository.Id, remoteBranchName)
		} else if strings.HasPrefix(remoteRef, tagPrefix) {
			remoteTagName := strings.TrimPrefix(remoteRef, tagPrefix)
			msg = gitopiaTypes.NewMsgDeleteTag(walletAddress.String(), h.remoteRepository.Id, remoteTagName)
		}

		err := signAndBroadcastTx(h.grpcConn, walletAddress.String(), h.chainId, privKey, msg)
		if err != nil {
			return "", err
		}

		return local, nil
	}

	remoteURL := fmt.Sprintf("%v/%v.git", objectsURL, h.remoteRepository.Id)
	remoteConfig := &goGitConfig.RemoteConfig{
		Name: "gitopia-objects-store",
		URLs: []string{remoteURL},
	}

	_, err = remote.Repo.CreateRemote(remoteConfig)
	if err != nil {
		return "", err
	}
	defer remote.Repo.DeleteRemote("gitopia-objects-store")

	force := false
	if strings.HasPrefix(local, "+") {
		local = strings.TrimPrefix(local, "+")
		force = true
	}

	pushOptions := &git.PushOptions{
		RemoteName: "gitopia-objects-store",
		RefSpecs:   []goGitConfig.RefSpec{goGitConfig.RefSpec(fmt.Sprintf("%s:%s", local, remoteRef))},
		Progress:   os.Stdout,
		Force:      force,
	}

	err = remote.Repo.Push(pushOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("fatal: error pushing the git objects, %v", err.Error())
	}

	var newRemoteRefSha, prevRemoteRefSha string

	// Update ref on gitopia
	if strings.HasPrefix(local, branchPrefix) {
		localCommitHash, err := remote.Repo.ResolveRevision(plumbing.Revision(local))
		if err != nil {
			return "", fmt.Errorf("fatal: local branch %s doesn't exist", local)
		}
		newRemoteRefSha = localCommitHash.String()

		remoteBranchName := strings.TrimPrefix(remoteRef, branchPrefix)
		branchShaResponse, err := h.queryClient.BranchSha(context.Background(), &gitopiaTypes.QueryGetBranchShaRequest{
			RepositoryId: h.remoteRepository.Id,
			BranchName:   remoteBranchName,
		})
		if err == nil {
			prevRemoteRefSha = branchShaResponse.Sha
		}

		msg = gitopiaTypes.NewMsgCreateBranch(walletAddress.String(), h.remoteRepository.Id, remoteBranchName, newRemoteRefSha)
	} else if strings.HasPrefix(local, tagPrefix) {
		localTagName := strings.TrimPrefix(local, tagPrefix)
		ref, err := remote.Repo.Tag(localTagName)
		if err != nil {
			return "", fmt.Errorf("fatal: invalid tag name, %v", localTagName)
		}
		newRemoteRefSha = ref.Hash().String()

		remoteTagName := strings.TrimPrefix(remoteRef, tagPrefix)
		tagShaResponse, err := h.queryClient.TagSha(context.Background(), &gitopiaTypes.QueryGetTagShaRequest{
			RepositoryId: h.remoteRepository.Id,
			TagName:      remoteTagName,
		})
		if err == nil {
			prevRemoteRefSha = tagShaResponse.Sha
		}

		msg = gitopiaTypes.NewMsgCreateTag(walletAddress.String(), h.remoteRepository.Id, remoteTagName, newRemoteRefSha)
	} else {
		return "", fmt.Errorf("fatal: not a valid branch/tag, %v", local)
	}

	err = signAndBroadcastTx(h.grpcConn, walletAddress.String(), h.chainId, privKey, msg)
	if err != nil {
		return "", err
	}

	_ = prevRemoteRefSha

	// Queue task to upload objects to arweave
	// saveToArweavePostBody := SaveToArweavePostBody{
	// 	RepositoryID:     h.remoteRepository.Id,
	// 	RemoteRefName:    remoteRef,
	// 	NewRemoteRefSha:  newRemoteRefSha,
	// 	PrevRemoteRefSha: prevRemoteRefSha,
	// }

	// postBody, err := json.Marshal(saveToArweavePostBody)
	// if err != nil {
	// 	return "", fmt.Errorf("fatal: failed to serialize post data: %v", err.Error())
	// }
	// responseBody := bytes.NewBuffer(postBody)
	// resp, err := http.Post(saveToArweaveURL, "application/json", responseBody)
	// if err != nil {
	// 	return "", fmt.Errorf("fatal: error posting saveToArweave: %v", err.Error())
	// }
	// defer resp.Body.Close()

	// if resp.StatusCode != http.StatusOK {
	// 	return "", fmt.Errorf("fatal: error saving to Arweave")
	// }

	return local, nil
}

func (h *GitopiaHandler) havePushPermission(walletAddress string) (bool, error) {
	var o gitopiaTypes.Organization

	if h.remoteRepository.Owner.Type == gitopiaTypes.RepositoryOwner_ORGANIZATION {
		res, err := h.queryClient.Organization(context.Background(), &gitopiaTypes.QueryGetOrganizationRequest{
			Id: h.remoteRepository.Owner.Id,
		})
		if err != nil {
			return false, fmt.Errorf("fatal: organization doesn't exist")
		}

		o = *res.Organization
	}

	return utils.HaveBranchPermission(h.remoteRepository, walletAddress, o), nil
}
