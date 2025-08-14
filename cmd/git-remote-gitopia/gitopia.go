package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	"github.com/gitopia/git-remote-gitopia/config"
	core "github.com/gitopia/git-remote-gitopia/core"
	"github.com/gitopia/git-remote-gitopia/core/api"
	"github.com/gitopia/git-remote-gitopia/core/wallet"
	gitopiatypes "github.com/gitopia/gitopia/v6/x/gitopia/types"
	"github.com/gitopia/gitopia/v6/x/gitopia/utils"
	storagetypes "github.com/gitopia/gitopia/v6/x/storage/types"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	AppName              = "git-remote-gitopia"
	AccountAddressPrefix = "gitopia"
	AccountPubKeyPrefix  = AccountAddressPrefix + sdk.PrefixPublic
	branchPrefix         = "refs/heads/"
	tagPrefix            = "refs/tags/"
)

type GitopiaHandler struct {
	grpcConn       *grpc.ClientConn
	queryClient    gitopiatypes.QueryClient
	feegrantClient feegrant.QueryClient
	bankClient     banktypes.QueryClient
	storageClient  storagetypes.QueryClient

	chainId              string
	remoteUserId         string
	remoteRepositoryName string
	remoteRepository     gitopiatypes.Repository

	didPush bool

	wallet wallet.Wallet
}

func (h *GitopiaHandler) Initialize(remote *core.Remote) error {
	var err error

	// Check existing configuration
	grpcHost, _ := config.GitConfigGet(config.GitopiaConfigGRPCHostOption)
	tmAddr, _ := config.GitConfigGet(config.GitopiaConfigTmAddrOption)

	// Validate and configure gRPC and RPC hosts
	needsReconfiguration := false
	if grpcHost == "" || tmAddr == "" || !api.CheckGRPCHostLiveness(grpcHost) || !api.CheckRPCHostLiveness(tmAddr) {
		needsReconfiguration = true
	}

	if needsReconfiguration {
		remote.Logger.Printf("Configuring Gitopia hosts...")
		provider := api.GetBestApiProvider()
		grpcHost = provider.GRPCHost
		tmAddr = provider.TMAddr

		if err := api.SetConfiguredGRPCHost(provider.GRPCHost); err != nil {
			return err
		}

		if err := api.SetConfiguredTmAddr(provider.TMAddr); err != nil {
			return err
		}
	}

	// Check and configure Git server host
	gitServerHost, _ := config.GitConfigGet(config.GitopiaConfigGitServerHostOption)

	if gitServerHost == "" || !api.CheckGitServerHostLiveness(gitServerHost) {
		gitServerHost = api.GetBestGitServerHost(grpcHost)
		if gitServerHost != "" {
			if err := api.SetConfiguredGitServerHost(gitServerHost); err != nil {
				return err
			}
		}
	}

	h.grpcConn, err = grpc.Dial(grpcHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	authtypes.RegisterInterfaces(interfaceRegistry)
	cryptocodec.RegisterInterfaces(interfaceRegistry)

	h.queryClient = gitopiatypes.NewQueryClient(h.grpcConn)
	serviceClient := tmservice.NewServiceClient(h.grpcConn)
	h.feegrantClient = feegrant.NewQueryClient(h.grpcConn)
	h.bankClient = banktypes.NewQueryClient(h.grpcConn)
	h.storageClient = storagetypes.NewQueryClient(h.grpcConn)

	nodeInfoRes, err := serviceClient.GetNodeInfo(context.Background(), &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return err
	}
	h.chainId = nodeInfoRes.DefaultNodeInfo.Network

	res, err := h.queryClient.AnyRepository(context.Background(), &gitopiatypes.QueryGetAnyRepositoryRequest{
		Id:             h.remoteUserId,
		RepositoryName: h.remoteRepositoryName,
	})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("error getting repository %s/%s", h.remoteUserId, h.remoteRepositoryName))
	}

	h.remoteRepository = *res.Repository

	// Configure LFS URL for clone operations to avoid SSH to non-existent "gitopia" hostname
	lfsURL := fmt.Sprintf("%v/%v.git", gitServerHost, h.remoteRepository.Id)
	cmd := core.GitCommand("git", "config", "--local", "lfs.url", lfsURL)
	if err := cmd.Run(); err != nil {
		// Log but don't fail if LFS config fails (repo might not have LFS)
		remote.Logger.Printf("Warning: could not configure LFS URL: %v", err)
	}

	return nil
}

func (h *GitopiaHandler) List(remote *core.Remote, forPush bool) ([]string, error) {
	out := make([]string, 0)

	branchAllRes, err := h.queryClient.RepositoryBranchAll(context.Background(), &gitopiatypes.QueryAllRepositoryBranchRequest{
		Id:             h.remoteRepository.Owner.Id,
		RepositoryName: h.remoteRepository.Name,
		Pagination: &query.PageRequest{
			Limit: math.MaxUint64,
		},
	})
	if err != nil {
		return out, err
	}
	for _, branch := range branchAllRes.Branch {
		out = append(out, fmt.Sprintf("%s %s%s", branch.Sha, branchPrefix, branch.Name))
	}

	tagAllRes, err := h.queryClient.RepositoryTagAll(context.Background(), &gitopiatypes.QueryAllRepositoryTagRequest{
		Id:             h.remoteRepository.Owner.Id,
		RepositoryName: h.remoteRepository.Name,
		Pagination: &query.PageRequest{
			Limit: math.MaxUint64,
		},
	})
	if err != nil {
		return out, err
	}
	for _, tag := range tagAllRes.Tag {
		out = append(out, fmt.Sprintf("%s %s%s", tag.Sha, tagPrefix, tag.Name))
	}

	out = append(out, fmt.Sprintf("@refs/heads/%s HEAD", h.remoteRepository.DefaultBranch))

	return out, nil
}

func (h *GitopiaHandler) Fetch(remote *core.Remote, refsToFetch []core.RefToFetch) error {
	gitServerHost, err := config.GitConfigGet(config.GitopiaConfigGitServerHostOption)
	if err != nil {
		return err
	}
	remoteURL := fmt.Sprintf("%v/%v.git", gitServerHost, h.remoteRepository.Id)
	lfsURL := remoteURL // Use same URL for LFS

	if !remote.Force {
		args := []string{
			"-c", fmt.Sprintf("lfs.url=%s", lfsURL),
			"fetch",
			"--no-write-fetch-head",
			remoteURL,
		}
		for _, ref := range refsToFetch {
			args = append(args, ref.Ref)
		}
		cmd := core.GitCommand("git", args...)
		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "error fetching from remote repository")
		}

		return nil
	}

	for _, ref := range refsToFetch {
		force := false
		if strings.HasPrefix(ref.Ref, "+") {
			ref.Ref = strings.TrimPrefix(ref.Ref, "+")
			force = true
		}

		args := []string{
			"-c", fmt.Sprintf("lfs.url=%s", lfsURL),
			"fetch",
			"--no-write-fetch-head",
			remoteURL,
			ref.Ref,
		}
		if force {
			args = append(args, "--force")
		}
		cmd := core.GitCommand("git", args...)
		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "error fetching from remote repository")
		}
	}

	return nil
}

func (h *GitopiaHandler) Push(remote *core.Remote, refsToPush []core.RefToPush) ([]string, error) {
	var err error

	if h.wallet == nil {
		h.wallet, err = wallet.InitWallet(h.bankClient, h.feegrantClient)
		if err != nil {
			return nil, err
		}
	}

	switch h.wallet.Type() {
	case wallet.ENV_VAR:
		remote.Logger.Printf("Loaded Gitopia wallet file, path: %s, address: %s\n", os.Getenv("GITOPIA_WALLET"), h.wallet.Address())
	case wallet.GITHUB_SEC:
		remote.Logger.Printf("Loaded Gitopia wallet from GitHub secret, wallet address: %s\n", h.wallet.Address())
	case wallet.LEDGER:
		remote.Logger.Printf("Using Ledger device, wallet address: %s\n", h.wallet.Address())
	case wallet.KEYRING_BACKEND:
		keyName := h.wallet.(wallet.OSKeyring).KeyName()
		remote.Logger.Printf("Using OS keyring, key name: %s, wallet address: %s\n", keyName, h.wallet.Address())
	default:
		return nil, errors.New("fatal: Unsupported wallet type")
	}

	havePushPermission, err := h.havePushPermission(h.wallet.Address())
	if err != nil {
		return nil, err
	}
	if !havePushPermission {
		return nil, fmt.Errorf("fatal: you don't have write permissions to this repository")
	}

	gitServerHost, err := config.GitConfigGet(config.GitopiaConfigGitServerHostOption)
	if err != nil {
		return nil, err
	}
	remoteURL := fmt.Sprintf("%v/%v.git", gitServerHost, h.remoteRepository.Id)
	lfsURL := remoteURL // Use same URL for LFS

	var pushRefspecs []string
	var deleteBranches, deleteTags []string
	var setBranches []gitopiatypes.MsgMultiSetBranch_Branch
	var setTags []gitopiatypes.MsgMultiSetTag_Tag
	isForce := false
	var res []string // To return the refs that were processed
	var packfileCid string

	packfileRes, err := h.storageClient.RepositoryPackfile(context.Background(), &storagetypes.QueryRepositoryPackfileRequest{
		RepositoryId: h.remoteRepository.Id,
	})
	if err == nil {
		packfileCid = packfileRes.Packfile.Cid
	}

	// --- First Pass: Collect refspecs and deletions ---
	for _, ref := range refsToPush {
		if ref.Local == "" { // This is a delete operation
			if strings.HasPrefix(ref.Remote, branchPrefix) {
				remoteBranchName := strings.TrimPrefix(ref.Remote, branchPrefix)
				if remoteBranchName == h.remoteRepository.DefaultBranch {
					return nil, fmt.Errorf("fatal: cannot delete default branch, %v", remoteBranchName)
				}
				deleteBranches = append(deleteBranches, remoteBranchName)
				res = append(res, ref.Remote)
			} else if strings.HasPrefix(ref.Remote, tagPrefix) {
				remoteTagName := strings.TrimPrefix(ref.Remote, tagPrefix)
				deleteTags = append(deleteTags, remoteTagName)
				res = append(res, ref.Remote)
			}
			continue
		}

		// This is a create/update operation
		localRef := ref.Local
		if strings.HasPrefix(localRef, "+") {
			localRef = strings.TrimPrefix(localRef, "+")
			isForce = true
		}

		pushRefspecs = append(pushRefspecs, fmt.Sprintf("%s:%s", ref.Local, ref.Remote))
	}

	// --- Execute a single Git Push if there's anything to push ---
	if len(pushRefspecs) > 0 {
		if h.wallet.Type() == wallet.LEDGER {
			remote.Logger.Println("Please sign the git server request on your ledger device.")
		}

		data := []byte("test")
		signature, err := h.wallet.SignData(data)
		if err != nil {
			return nil, errors.Wrap(err, "error signing data")
		}
		credential := fmt.Sprintf("%s:%s", h.wallet.Address(), signature)

		args := []string{
			"-c", fmt.Sprintf("http.extraheader=Authorization: Basic %s", base64.StdEncoding.EncodeToString([]byte(credential))),
			"-c", "credential.helper=",
			"-c", "credential.helper=gitopia",
			"-c", fmt.Sprintf("lfs.url=%s", lfsURL),
			"push",
			"--no-verify", // Keep this to prevent the double-hook call
			remoteURL,
		}

		// Add all refspecs to the command
		args = append(args, pushRefspecs...)

		if isForce {
			args = append(args, "--force")
		}

		cmd := core.GitCommand("git", args...)
		if err := cmd.Run(); err != nil {
			return nil, errors.Wrap(err, "error pushing to remote repository")
		}
	}

	// --- Second Pass: Collect metadata for Gitopia transaction ---
	for _, ref := range refsToPush {
		if ref.Local == "" {
			continue // Deletes already handled
		}

		localRef := ref.Local
		if strings.HasPrefix(localRef, "+") {
			localRef = strings.TrimPrefix(localRef, "+")
		}

		// Update ref on gitopia
		if strings.HasPrefix(localRef, branchPrefix) {
			localCommitHash, err := remote.Repo.ResolveRevision(plumbing.Revision(localRef))
			if err != nil {
				return nil, fmt.Errorf("fatal: local branch %s doesn't exist", localRef)
			}

			remoteBranchName := strings.TrimPrefix(ref.Remote, branchPrefix)
			branch := gitopiatypes.MsgMultiSetBranch_Branch{
				Name: remoteBranchName,
				Sha:  localCommitHash.String(),
			}
			setBranches = append(setBranches, branch)
			res = append(res, ref.Remote)
		} else if strings.HasPrefix(localRef, tagPrefix) {
			localTagName := strings.TrimPrefix(localRef, tagPrefix)
			tagRef, err := remote.Repo.Tag(localTagName)
			if err != nil {
				// Could be a lightweight tag, resolve revision instead
				commitHash, err := remote.Repo.ResolveRevision(plumbing.Revision(localRef))
				if err != nil {
					return nil, fmt.Errorf("fatal: invalid tag name or ref, %v", localTagName)
				}
				tag := gitopiatypes.MsgMultiSetTag_Tag{
					Name: strings.TrimPrefix(ref.Remote, tagPrefix),
					Sha:  commitHash.String(),
				}
				setTags = append(setTags, tag)
			} else {
				// Annotated tag
				tag := gitopiatypes.MsgMultiSetTag_Tag{
					Name: strings.TrimPrefix(ref.Remote, tagPrefix),
					Sha:  tagRef.Hash().String(),
				}
				setTags = append(setTags, tag)
			}

			res = append(res, ref.Remote)
		} else {
			return nil, fmt.Errorf("fatal: invalid refspec, %v", ref)
		}
	}

	var msg []sdk.Msg

	// Approve packfile update
	packfileUpdateProposalRes, err := h.storageClient.PackfileUpdateProposal(context.Background(), &storagetypes.QueryPackfileUpdateProposalRequest{
		RepositoryId: h.remoteRepository.Id,
		User:         h.wallet.Address(),
	})
	if err != nil {
		return nil, err
	}
	msg = append(msg, storagetypes.NewMsgApproveRepositoryPackfileUpdate(h.wallet.Address(), packfileUpdateProposalRes.PackfileUpdateProposal.Id))

	lfsObjectUpdateProposalRes, err := h.storageClient.LFSObjectUpdateProposalsByRepositoryId(context.Background(), &storagetypes.QueryLFSObjectUpdateProposalsByRepositoryIdRequest{
		RepositoryId: h.remoteRepository.Id,
		User:         h.wallet.Address(),
	})
	if err != nil {
		return nil, err
	}

	// Approve LFS object update
	for _, lfsObjectUpdateProposal := range lfsObjectUpdateProposalRes.LfsObjectProposals {
		msg = append(msg, storagetypes.NewMsgApproveLFSObjectUpdate(h.wallet.Address(), lfsObjectUpdateProposal.Id))
	}

	if len(setBranches) > 0 {
		msg = append(msg, gitopiatypes.NewMsgMultiSetBranch(h.wallet.Address(), gitopiatypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, setBranches, packfileCid))
	}
	if len(setTags) > 0 {
		msg = append(msg, gitopiatypes.NewMsgMultiSetTag(h.wallet.Address(), gitopiatypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, setTags, packfileCid))
	}
	if len(deleteBranches) > 0 {
		msg = append(msg, gitopiatypes.NewMsgMultiDeleteBranch(h.wallet.Address(), gitopiatypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, deleteBranches))
	}
	if len(deleteTags) > 0 {
		msg = append(msg, gitopiatypes.NewMsgMultiDeleteTag(h.wallet.Address(), gitopiatypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, deleteTags))
	}

	if h.wallet.Type() == wallet.LEDGER {
		remote.Logger.Println("Please sign the gitopia transaction on your ledger device.")
	}

	if err := h.wallet.SignAndBroadcast(h.grpcConn, msg); err != nil {
		return nil, err
	}

	return res, nil
}

func (h *GitopiaHandler) havePushPermission(walletAddress string) (havePermission bool, err error) {
	if h.remoteRepository.Owner.Type == gitopiatypes.OwnerType_USER {
		if h.wallet.Address() == h.remoteRepository.Owner.Id {
			havePermission = true
		}
	} else if h.remoteRepository.Owner.Type == gitopiatypes.OwnerType_DAO {
		resp, err := h.queryClient.DaoMemberAll(context.Background(), &gitopiatypes.QueryAllDaoMemberRequest{
			DaoId: h.remoteRepository.Owner.Id,
		})
		if err != nil {
			return havePermission, errors.Wrap(err, "error querying DAO members")
		}

		for _, member := range resp.Members {
			if member.Member.Address == walletAddress {
				havePermission = true
				break
			}
		}
	}

	if !havePermission {
		if i, exists := utils.RepositoryCollaboratorExists(h.remoteRepository.Collaborators, h.wallet.Address()); exists {
			if h.remoteRepository.Collaborators[i].Permission >= gitopiatypes.PushBranchPermission {
				havePermission = true
			}
		}
	}

	return havePermission, nil
}
