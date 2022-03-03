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
	"github.com/gitopia/git-remote-gitopia/config"
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
	saveToArweaveURL     = "http://35.200.147.237:5000/save"
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

	h.grpcConn, err = grpc.Dial(config.GRPCHost,
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
	remoteURL := fmt.Sprintf("%v/%v.git", config.GitServerHost, h.remoteRepository.Id)
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
		Tags:       git.TagMode(3),
	}

	err = remote.Repo.Fetch(fetchOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fatal: %v", err.Error())
	}

	return nil
}

func (h *GitopiaHandler) Push(remote *core.Remote, refsToPush []core.RefToPush) (*[]string, error) {
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
			return nil, fmt.Errorf("fatal: GITOPIA_WALLET environment variable is not set")
		}

		var err error
		buffer, err = os.ReadFile(gitopiaWalletPath)
		if err != nil {
			return nil, fmt.Errorf("fatal: error reading gitopia wallet")
		}

	}

	err := json.Unmarshal(buffer, &gitopiaWallet)
	if err != nil {
		return nil, fmt.Errorf("fatal: error decoding wallet file")
	}

	// Generate private key
	hdPath := gitopiaWallet.HDpath + strconv.Itoa(gitopiaWallet.PathIncrement)
	derivedPriv, err := hd.Secp256k1.Derive()(gitopiaWallet.Mnemonic, "", hdPath)
	if err != nil {
		return nil, err
	}

	privKey := hd.Secp256k1.Generate()(derivedPriv)
	walletAddress := sdk.AccAddress(privKey.PubKey().Address())

	havePushPermission, err := h.havePushPermission(walletAddress.String())
	if err != nil {
		return nil, err
	}
	if !havePushPermission {
		return nil, fmt.Errorf("fatal: you don't have write permissions to this repository")
	}

	var msg []sdk.Msg

	// Delete branch/tag

	remoteURL := fmt.Sprintf("%v/%v.git", config.GitServerHost, h.remoteRepository.Id)
	remoteConfig := &goGitConfig.RemoteConfig{
		Name: "gitopia-objects-store",
		URLs: []string{remoteURL},
	}

	_, err = remote.Repo.CreateRemote(remoteConfig)
	if err != nil {
		return nil, err
	}
	defer remote.Repo.DeleteRemote("gitopia-objects-store")

	var newRemoteRefSha, prevRemoteRefSha string
	var setBranches []*gitopiaTypes.MsgMultiSetRepositoryBranch_Branch
	var setTags []*gitopiaTypes.MsgMultiSetRepositoryTag_Tag
	var deleteBranches, deleteTags []string
	var res []string

	for _, ref := range refsToPush {
		if ref.Local == "" {
			if strings.HasPrefix(ref.Remote, branchPrefix) {
				remoteBranchName := strings.TrimPrefix(ref.Remote, branchPrefix)

				// Check if it's the default branch
				if remoteBranchName == h.remoteRepository.DefaultBranch {
					return nil, fmt.Errorf("fatal: cannot delete default branch, %v", remoteBranchName)
				}

				deleteBranches = append(deleteBranches, remoteBranchName)
				res = append(res, ref.Remote)
			} else if strings.HasPrefix(refsToPush[0].Remote, tagPrefix) {
				remoteTagName := strings.TrimPrefix(refsToPush[0].Remote, tagPrefix)
				deleteTags = append(deleteTags, remoteTagName)
				res = append(res, ref.Remote)
			}

			continue
		}

		force := false
		if strings.HasPrefix(ref.Local, "+") {
			ref.Local = strings.TrimPrefix(ref.Local, "+")
			force = true
		}

		pushOptions := &git.PushOptions{
			RemoteName: "gitopia-objects-store",
			RefSpecs:   []goGitConfig.RefSpec{goGitConfig.RefSpec(fmt.Sprintf("%s:%s", ref.Local, ref.Remote))},
			Progress:   os.Stdout,
			Force:      force,
		}

		err = remote.Repo.Push(pushOptions)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return nil, fmt.Errorf("fatal: error pushing the git objects, %v", err.Error())
		}

		// Update ref on gitopia
		if strings.HasPrefix(ref.Local, branchPrefix) {
			localCommitHash, err := remote.Repo.ResolveRevision(plumbing.Revision(ref.Local))
			if err != nil {
				return nil, fmt.Errorf("fatal: local branch %s doesn't exist", ref.Local)
			}
			newRemoteRefSha = localCommitHash.String()

			remoteBranchName := strings.TrimPrefix(ref.Remote, branchPrefix)
			branchShaResponse, err := h.queryClient.BranchSha(context.Background(), &gitopiaTypes.QueryGetBranchShaRequest{
				RepositoryId: h.remoteRepository.Id,
				BranchName:   remoteBranchName,
			})
			if err == nil {
				prevRemoteRefSha = branchShaResponse.Sha
			}

			branch := gitopiaTypes.MsgMultiSetRepositoryBranch_Branch{
				Name:      remoteBranchName,
				CommitSHA: newRemoteRefSha,
			}

			setBranches = append(setBranches, &branch)
			res = append(res, ref.Remote)
		} else if strings.HasPrefix(ref.Local, tagPrefix) {
			localTagName := strings.TrimPrefix(ref.Local, tagPrefix)
			tagRef, err := remote.Repo.Tag(localTagName)
			if err != nil {
				return nil, fmt.Errorf("fatal: invalid tag name, %v", localTagName)
			}
			newRemoteRefSha = tagRef.Hash().String()

			remoteTagName := strings.TrimPrefix(ref.Remote, tagPrefix)
			tagShaResponse, err := h.queryClient.TagSha(context.Background(), &gitopiaTypes.QueryGetTagShaRequest{
				RepositoryId: h.remoteRepository.Id,
				TagName:      remoteTagName,
			})
			if err == nil {
				prevRemoteRefSha = tagShaResponse.Sha
			}

			tag := gitopiaTypes.MsgMultiSetRepositoryTag_Tag{
				Name:      remoteTagName,
				CommitSHA: newRemoteRefSha,
			}

			setTags = append(setTags, &tag)
			res = append(res, ref.Remote)
		} else {
			return nil, fmt.Errorf("fatal: not a valid branch/tag, %v", ref.Local)
		}
	}

	if len(setBranches) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiSetRepositoryBranch(walletAddress.String(), h.remoteRepository.Id, setBranches))
	}
	if len(setTags) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiSetRepositoryTag(walletAddress.String(), h.remoteRepository.Id, setTags))
	}
	if len(deleteBranches) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiDeleteBranch(walletAddress.String(), h.remoteRepository.Id, deleteBranches))
	}
	if len(deleteTags) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiDeleteTag(walletAddress.String(), h.remoteRepository.Id, deleteTags))
	}

	err = signAndBroadcastTx(h.grpcConn, walletAddress.String(), h.chainId, privKey, msg)
	if err != nil {
		return nil, err
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

	return &res, nil
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
