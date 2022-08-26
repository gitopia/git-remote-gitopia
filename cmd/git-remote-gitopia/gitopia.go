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
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/ledger"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gitopia/git-remote-gitopia/config"
	core "github.com/gitopia/git-remote-gitopia/core"
	gitopiaTypes "github.com/gitopia/gitopia/x/gitopia/types"
	"github.com/gitopia/gitopia/x/gitopia/utils"
	offchaintypes "github.com/gitopia/gitopia/x/offchain/types"
	"github.com/go-git/go-git/v5"
	goGitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gogittransporthttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/pkg/errors"
	"github.com/tendermint/starport/starport/pkg/cosmosaccount"
	"github.com/tendermint/starport/starport/pkg/cosmosclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	AppName                    = "git-remote-gitopia"
	AccountAddressPrefix       = "gitopia"
	AccountPubKeyPrefix        = AccountAddressPrefix + sdk.PrefixPublic
	gitopiaConfigSection       = "gitopia"
	gitopiaConfigKeyOption     = "key"
	gitopiaConfigBackendOption = "backend"
	saveToArweaveURL           = "http://35.200.147.237:5000/save"
	branchPrefix               = "refs/heads/"
	tagPrefix                  = "refs/tags/"
	defaultFees                = "200utlore"
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

func (w *GitopiaWallet) walletAddress() (string, error) {
	privKey, err := w.privKey()
	if err != nil {
		return "", err
	}
	walletAddress := sdk.AccAddress(privKey.PubKey().Address())

	return walletAddress.String(), nil
}

func (w *GitopiaWallet) privKey() (cryptotypes.PrivKey, error) {
	// Generate private key
	hdPath := w.HDpath + strconv.Itoa(w.PathIncrement)
	derivedPriv, err := hd.Secp256k1.Derive()(w.Mnemonic, "", hdPath)
	if err != nil {
		return nil, err
	}

	privKey := hd.Secp256k1.Generate()(derivedPriv)
	return privKey, nil
}

type SaveToArweavePostBody struct {
	RepositoryID     uint64 `json:"repository_id"`
	RemoteRefName    string `json:"remote_ref_name"`
	NewRemoteRefSha  string `json:"new_remote_ref_sha"`
	PrevRemoteRefSha string `json:"prev_remote_ref_sha"`
}

type secretType int

const (
	UNKNOWN secretType = iota
	ENV_VAR
	LEDGER
	KEYRING_BACKEND
	GITHIB_SEC
)

type keyringBackend struct {
	key     string
	backend string
	cc      cosmosclient.Client
}

func newKeyringBackend(k string, b string, c cosmosclient.Client) keyringBackend {
	c.Factory = c.Factory.WithFees(defaultFees)
	return keyringBackend{
		key:     k,
		backend: b,
		cc:      c,
	}
}

func (k keyringBackend) address() (sdk.Address, error) {
	return k.cc.Address(k.key)
}

type GitopiaHandler struct {
	grpcConn    *grpc.ClientConn
	queryClient gitopiaTypes.QueryClient

	chainId              string
	remoteUserId         string
	remoteRepositoryName string
	remoteRepository     gitopiaTypes.Repository

	didPush bool

	secType          secretType
	gWallet          *GitopiaWallet            // ENV_VAR
	kb               keyringBackend            // KEYRING_BACKEND
	ledgerPrivateKey cryptotypes.LedgerPrivKey // LEDGER
}

func (h *GitopiaHandler) Initialize(remote *core.Remote) error {
	var err error

	h.grpcConn, err = grpc.Dial(config.GRPCHost,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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

func (h *GitopiaHandler) initGitopiaWallet() (string, error) {
	var gitopiaWallet GitopiaWallet
	var buffer []byte
	var err error
	h.didPush = true

	// Read wallet
	isGitHubAction := os.Getenv("GITHUB_ACTIONS")
	if isGitHubAction == "true" {
		// Read from GitHub secret
		buffer = []byte(os.Getenv("GITOPIA_WALLET"))
		h.secType = GITHIB_SEC
	} else if len(os.Getenv("GITOPIA_WALLET")) != 0 {
		gitopiaWalletPath := os.Getenv("GITOPIA_WALLET")
		buffer, err = os.ReadFile(gitopiaWalletPath)
		if err != nil {
			return "", fmt.Errorf("fatal: error reading gitopia wallet")
		}
		h.secType = ENV_VAR
	} else {
		return "", fmt.Errorf("fatal: GITOPIA_WALLET environment variable is not set")
	}

	err = json.Unmarshal(buffer, &gitopiaWallet)
	if err != nil {
		return "", fmt.Errorf("fatal: error decoding wallet file")
	}

	h.gWallet = &gitopiaWallet
	walletAddress, err := h.gWallet.walletAddress()
	if err != nil {
		return "", err
	}
	return walletAddress, nil
}

func (h *GitopiaHandler) initGitopiaKey() (string, error) {
	var key string
	var backend string
	var cc cosmosclient.Client
	conf, err := goGitConfig.LoadConfig(goGitConfig.GlobalScope)
	if err != nil {
		return "", err
	}
	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigKeyOption) {
		key = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigKeyOption)
	} else {
		return "", errors.New("gitopia key not configured")
	}

	if conf.Raw.HasSection(gitopiaConfigSection) &&
		conf.Raw.Section(gitopiaConfigSection).HasOption(gitopiaConfigBackendOption) {
		backend = conf.Raw.Section(gitopiaConfigSection).Option(gitopiaConfigBackendOption)
	} else {
		backend = keyring.BackendOS // default to OS. same as cosmos keys subcommand
	}

	h.secType = KEYRING_BACKEND
	cc, err = cosmosclient.New(context.Background(),
		cosmosclient.WithNodeAddress(config.TmAddr),
		// same service name used in both helper and keys management app
		cosmosclient.WithKeyringServiceName(AppName),                           // not suported on macos
		cosmosclient.WithKeyringBackend(cosmosaccount.KeyringBackend(backend)), // not all backends supported by cosmos are supported by cosmos client
		cosmosclient.WithAddressPrefix(AccountAddressPrefix),
	)
	if err != nil {
		return "", err
	}
	h.kb = newKeyringBackend(key, backend, cc)

	wa, err := h.kb.address()
	if err != nil {
		return "", err
	}
	return wa.String(), nil
}

func (h *GitopiaHandler) initLedgerSecret() (string, error) {
	ledgerPrivKey, err := ledger.NewPrivKeySecp256k1Unsafe(hd.BIP44Params{
		Purpose:      44,
		CoinType:     118,
		Account:      0,
		Change:       false,
		AddressIndex: 0,
	})
	if err != nil {
		return "", err
	}
	h.ledgerPrivateKey = ledgerPrivKey
	h.secType = LEDGER
	walletAddress := sdk.AccAddress(ledgerPrivKey.PubKey().Address())
	return walletAddress.String(), nil
}

func (h *GitopiaHandler) initSecrets() (string, error) {
	walletAddress := ""
	var err error
	if h.secType == UNKNOWN {
		walletAddress, err = h.initGitopiaKey()
		if err != nil {
			walletAddress, err = h.initGitopiaWallet()
			if err != nil {
				walletAddress, err = h.initLedgerSecret()
				if err != nil {
					return "", fmt.Errorf("fatal: Gitopia wallet is not configured! Set gitopia key or use Ledger")
				}
			}
		}
	}
	return walletAddress, nil
}

func (h *GitopiaHandler) Push(remote *core.Remote, refsToPush []core.RefToPush) (*[]string, error) {
	walletAddress, err := h.initSecrets()
	if err != nil {
		return nil, err
	}

	havePushPermission, err := h.havePushPermission(walletAddress)
	if err != nil {
		return nil, err
	}
	if !havePushPermission {
		return nil, fmt.Errorf("fatal: you don't have write permissions to this repository")
	}

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

		encConf := simapp.MakeTestEncodingConfig()
		offchaintypes.RegisterInterfaces(encConf.InterfaceRegistry)
		offchaintypes.RegisterLegacyAminoCodec(encConf.Amino)

		var privKey offchaintypes.SignatureProvider

		if h.secType == KEYRING_BACKEND {
			// TODO
			// k, err := sdkkeyring.New(AppName, h.kb.backend, "", os.Stdin)
		} else if h.secType == LEDGER {
			privKey = h.ledgerPrivateKey
		} else {
			privKey, err = h.gWallet.privKey()
		}

		signer := offchaintypes.NewSigner(encConf.TxConfig, privKey)
		accAddress, err := sdk.AccAddressFromBech32(walletAddress)
		data := []byte("test")
		signData := offchaintypes.NewMsgSignData(accAddress, data)

		tx, err := signer.Sign([]sdk.Msg{signData})
		if err != nil {
			return nil, fmt.Errorf("fatal: error signing tx")
		}

		txBz, err := encConf.TxConfig.TxJSONEncoder()(tx)
		if err != nil {
			return nil, fmt.Errorf("fatal: error encoding tx, %s", err.Error())
		}

		auth := &gogittransporthttp.TokenAuth{
			Token: string(txBz),
		}

		pushOptions := &git.PushOptions{
			RemoteName: "gitopia-objects-store",
			RefSpecs:   []goGitConfig.RefSpec{goGitConfig.RefSpec(fmt.Sprintf("%s:%s", ref.Local, ref.Remote))},
			Progress:   os.Stdout,
			Force:      force,
			Auth:       auth,
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

	var msg []sdk.Msg

	if len(setBranches) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiSetRepositoryBranch(walletAddress, h.remoteRepository.Id, setBranches))
	}
	if len(setTags) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiSetRepositoryTag(walletAddress, h.remoteRepository.Id, setTags))
	}
	if len(deleteBranches) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiDeleteBranch(walletAddress, h.remoteRepository.Id, deleteBranches))
	}
	if len(deleteTags) > 0 {
		msg = append(msg, gitopiaTypes.NewMsgMultiDeleteTag(walletAddress, h.remoteRepository.Id, deleteTags))
	}

	if h.secType == KEYRING_BACKEND {
		txResp, err := h.kb.cc.BroadcastTx(h.kb.key, msg...)
		if err != nil {
			return nil, err
		}
		if txResp.TxResponse.Code != 0 {
			return nil, errors.WithMessage(err, "error broadcasting transaction")
		}

	} else {
		privKey, err := h.gWallet.privKey()
		if err != nil {
			return nil, err
		}
		err = signAndBroadcastTx(h.grpcConn, walletAddress, h.chainId, privKey, h.ledgerPrivateKey, msg, h.secType == LEDGER)
		if err != nil {
			return nil, err
		}
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
			return false, errors.WithMessage(err, "fatal: organization doesn't exist")
		}

		o = *res.Organization
	}

	return utils.HavePermission(h.remoteRepository, walletAddress, utils.PushBranchPermission, o), nil
}
