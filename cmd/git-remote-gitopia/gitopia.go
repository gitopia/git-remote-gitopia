package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/gitopia/git-remote-gitopia/config"
	core "github.com/gitopia/git-remote-gitopia/core"
	"github.com/gitopia/git-remote-gitopia/core/wallet"
	gitopiaTypes "github.com/gitopia/gitopia/x/gitopia/types"
	"github.com/gitopia/gitopia/x/gitopia/utils"
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
	grpcConn    *grpc.ClientConn
	queryClient gitopiaTypes.QueryClient

	chainId              string
	remoteUserId         string
	remoteRepositoryName string
	remoteRepository     gitopiaTypes.Repository

	didPush bool

	wallet wallet.Wallet
}

func (h *GitopiaHandler) Initialize(remote *core.Remote) error {
	var err error

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	authtypes.RegisterInterfaces(interfaceRegistry)
	cryptocodec.RegisterInterfaces(interfaceRegistry)

	h.grpcConn, err = grpc.Dial(config.GRPCHost,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(codec.NewProtoCodec(interfaceRegistry).GRPCCodec())),
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
	res, err := h.queryClient.AnyRepository(context.Background(), &gitopiaTypes.QueryGetAnyRepositoryRequest{
		Id:             h.remoteUserId,
		RepositoryName: h.remoteRepositoryName,
	})
	if err != nil {
		return err
	}

	h.remoteRepository = *res.Repository

	return nil
}

func (h *GitopiaHandler) List(remote *core.Remote, forPush bool) ([]string, error) {
	out := make([]string, 0)

	branchAllRes, err := h.queryClient.RepositoryBranchAll(context.Background(), &gitopiaTypes.QueryAllRepositoryBranchRequest{
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

	tagAllRes, err := h.queryClient.RepositoryTagAll(context.Background(), &gitopiaTypes.QueryAllRepositoryTagRequest{
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

func (h *GitopiaHandler) Fetch(remote *core.Remote, sha, ref string) error {
	remoteURL := fmt.Sprintf("%v/%v.git", config.GitServerHost, h.remoteRepository.Id)

	force := false
	if strings.HasPrefix(ref, "+") {
		ref = strings.TrimPrefix(ref, "+")
		force = true
	}

	args := []string{
		"fetch",
		remoteURL,
		ref,
	}
	if force {
		args = append(args, "--force")
	}
	cmd, pipe := core.GitCommand("git", args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	defer core.CleanUpProcessGroup(cmd)

	if _, err := io.Copy(ioutil.Discard, pipe); err != nil {
		return err
	}

	return nil
}

func (h *GitopiaHandler) Push(remote *core.Remote, refsToPush []core.RefToPush) (*[]string, error) {
	var err error

	if h.wallet == nil {
		h.wallet, err = wallet.InitWallet()
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

	remoteURL := fmt.Sprintf("%v/%v.git", config.GitServerHost, h.remoteRepository.Id)

	var newRemoteRefSha string
	var setBranches []gitopiaTypes.MsgMultiSetBranch_Branch
	var setTags []gitopiaTypes.MsgMultiSetTag_Tag
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

		data := []byte("test")
		signature, err := h.wallet.SignData(data)
		if err != nil {
			return nil, errors.Wrap(err, "error signing data")
		}

		if h.wallet.Type() == wallet.LEDGER {
			remote.Logger.Println("Please sign the git server request on your ledger device.")
		}

		args := []string{
			"-c",
			fmt.Sprintf("http.extraheader=Authorization: Bearer %s", signature),
			"push",
			remoteURL,
			fmt.Sprintf("%s:%s", ref.Local, ref.Remote),
		}
		if force {
			args = append(args, "--force")
		}
		cmd, _ := core.GitCommand("git", args...)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		defer core.CleanUpProcessGroup(cmd)

		// if _, err := io.Copy(os.Stderr, pipe); err != nil {
		// 	return nil, err
		// }

		// Update ref on gitopia
		if strings.HasPrefix(ref.Local, branchPrefix) {
			localCommitHash, err := remote.Repo.ResolveRevision(plumbing.Revision(ref.Local))
			if err != nil {
				return nil, fmt.Errorf("fatal: local branch %s doesn't exist", ref.Local)
			}

			newRemoteRefSha = localCommitHash.String()
			remoteBranchName := strings.TrimPrefix(ref.Remote, branchPrefix)
			branch := gitopiaTypes.MsgMultiSetBranch_Branch{
				Name: remoteBranchName,
				Sha:  newRemoteRefSha,
			}

			setBranches = append(setBranches, branch)
			res = append(res, ref.Remote)
		} else if strings.HasPrefix(ref.Local, tagPrefix) {
			localTagName := strings.TrimPrefix(ref.Local, tagPrefix)
			tagRef, err := remote.Repo.Tag(localTagName)
			if err != nil {
				return nil, fmt.Errorf("fatal: invalid tag name, %v", localTagName)
			}

			newRemoteRefSha = tagRef.Hash().String()
			remoteTagName := strings.TrimPrefix(ref.Remote, tagPrefix)
			tag := gitopiaTypes.MsgMultiSetTag_Tag{
				Name: remoteTagName,
				Sha:  newRemoteRefSha,
			}

			setTags = append(setTags, tag)
			res = append(res, ref.Remote)
		} else {
			return nil, fmt.Errorf("fatal: not a valid branch/tag, %v", ref.Local)
		}
	}

	var msg []sdk.Msg

	if len(setBranches) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiSetBranch(h.wallet.Address(), gitopiaTypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, setBranches))
	}
	if len(setTags) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiSetTag(h.wallet.Address(), gitopiaTypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, setTags))
	}
	if len(deleteBranches) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiDeleteBranch(h.wallet.Address(), gitopiaTypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, deleteBranches))
	}
	if len(deleteTags) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiDeleteTag(h.wallet.Address(), gitopiaTypes.RepositoryId{
			Id:   h.remoteRepository.Owner.Id,
			Name: h.remoteRepository.Name,
		}, deleteTags))
	}

	switch v := h.wallet.(type) {
	case wallet.OSKeyring:
		if err := v.SignAndBroadcast(msg); err != nil {
			return nil, err
		}
	case wallet.GitopiaWallet:
		if err := v.SignAndBroadcast(h.grpcConn, msg); err != nil {
			return nil, err
		}
	case wallet.Ledger:
		if err := v.SignAndBroadcast(h.grpcConn, msg); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown wallet type")
	}

	return &res, nil
}

func (h *GitopiaHandler) havePushPermission(walletAddress string) (havePermission bool, err error) {
	if h.remoteRepository.Owner.Type == gitopiaTypes.OwnerType_USER {
		if h.wallet.Address() == h.remoteRepository.Owner.Id {
			havePermission = true
		}
	} else if h.remoteRepository.Owner.Type == gitopiaTypes.OwnerType_DAO {
		member, err := h.queryClient.DaoMember(context.Background(), &gitopiaTypes.QueryGetDaoMemberRequest{
			DaoId:  h.remoteRepository.Owner.Id,
			UserId: h.wallet.Address(),
		})
		if err != nil {
			return havePermission, err
		}
		if member.Member.Role == gitopiaTypes.MemberRole_OWNER {
			havePermission = true
		}
	}

	if !havePermission {
		if i, exists := utils.RepositoryCollaboratorExists(h.remoteRepository.Collaborators, h.wallet.Address()); exists {
			if h.remoteRepository.Collaborators[i].Permission >= gitopiaTypes.PushBranchPermission {
				havePermission = true
			}
		}
	}

	return havePermission, nil
}
