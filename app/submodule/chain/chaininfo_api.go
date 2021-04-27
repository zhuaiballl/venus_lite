package chain

import (
	"context"
	"time"

	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	acrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"
	xerrors "github.com/pkg/errors"
)

type IChainInfo interface {
	BlockTime() time.Duration

	ChainList(ctx context.Context, tsKey types.TipSetKey, count int) ([]types.TipSetKey, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainSetHead(ctx context.Context, key types.TipSetKey) error
	ChainGetTipSet(key types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, height abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetRandomnessFromBeacon(ctx context.Context, key types.TipSetKey, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainGetRandomnessFromTickets(ctx context.Context, tsk types.TipSetKey, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainGetBlock(ctx context.Context, id cid.Cid) (*types.BlockHeader, error)
	ChainGetMessage(ctx context.Context, msgID cid.Cid) (*types.UnsignedMessage, error)
	ChainGetBlockMessages(ctx context.Context, bid cid.Cid) (*BlockMessages, error)
	ChainGetReceipts(ctx context.Context, id cid.Cid) ([]types.MessageReceipt, error)
	ChainGetParentMessages(ctx context.Context, bcid cid.Cid) ([]Message, error)
	ChainGetParentReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error)
	ChainNotify(ctx context.Context) chan []*chain.HeadChange

	ChainGetFullBlock(ctx context.Context, id cid.Cid) (*types.FullBlock, error)
	GetParentStateRootActor(ctx context.Context, ts *types.TipSet, addr address.Address) (*types.Actor, error)

	MessageWait(ctx context.Context, msgCid cid.Cid, confidence, lookback abi.ChainEpoch) (*chain.ChainMessage, error)

	ProtocolParameters(ctx context.Context) (*ProtocolParams, error)

	StateNetworkName(ctx context.Context) (NetworkName, error)
	StateSearchMsg(ctx context.Context, mCid cid.Cid) (*MsgLookup, error)
	StateWaitMsg(ctx context.Context, mCid cid.Cid, confidence abi.ChainEpoch) (*MsgLookup, error)
	StateGetReceipt(ctx context.Context, msg cid.Cid, tsk types.TipSetKey) (*types.MessageReceipt, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
}

var _ IChainInfo = &ChainInfoAPI{}

type ChainInfoAPI struct { //nolint
	chain *ChainSubmodule
}

//NewChainInfoAPI new chain info api
func NewChainInfoAPI(chain *ChainSubmodule) ChainInfoAPI {
	return ChainInfoAPI{chain: chain}
}

// BlockTime returns the block time
func (chainInfoAPI *ChainInfoAPI) BlockTime() time.Duration {
	return chainInfoAPI.chain.config.BlockTime()
}

// ChainLs returns an iterator of tipsets from specified head by tsKey to genesis
func (chainInfoAPI *ChainInfoAPI) ChainList(ctx context.Context, tsKey types.TipSetKey, count int) ([]types.TipSetKey, error) {
	fromTs, err := chainInfoAPI.chain.ChainReader.GetTipSet(tsKey)
	if err != nil {
		return nil, xerrors.Wrap(err, "could not retrieve network name")
	}
	tipset, err := chainInfoAPI.chain.ChainReader.Ls(ctx, fromTs, count)
	if err != nil {
		return nil, err
	}
	tipsetKey := make([]types.TipSetKey, len(tipset))
	for i, ts := range tipset {
		tipsetKey[i] = ts.Key()
	}
	return tipsetKey, nil
}

// ProtocolParameters return chain parameters
func (chainInfoAPI *ChainInfoAPI) ProtocolParameters(ctx context.Context) (*ProtocolParams, error) {
	networkName, err := chainInfoAPI.getNetworkName(ctx)
	if err != nil {
		return nil, xerrors.Wrap(err, "could not retrieve network name")
	}

	var supportedSectors []SectorInfo
	for proof := range miner0.SupportedProofTypes {
		size, err := proof.SectorSize()
		if err != nil {
			return nil, xerrors.Wrap(err, "could not retrieve network name")
		}
		maxUserBytes := abi.PaddedPieceSize(size).Unpadded()
		supportedSectors = append(supportedSectors, SectorInfo{size, maxUserBytes})
	}

	return &ProtocolParams{
		Network:          networkName,
		BlockTime:        chainInfoAPI.chain.config.BlockTime(),
		SupportedSectors: supportedSectors,
	}, nil
}

func (chainInfoAPI *ChainInfoAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return chainInfoAPI.chain.ChainReader.GetHead(), nil
}

// ChainSetHead sets `key` as the new head of this chain iff it exists in the nodes chain store.
func (chainInfoAPI *ChainInfoAPI) ChainSetHead(ctx context.Context, key types.TipSetKey) error {
	ts, err := chainInfoAPI.chain.ChainReader.GetTipSet(key)
	if err != nil {
		return err
	}
	return chainInfoAPI.chain.ChainReader.SetHead(ctx, ts)
}

// ChainTipSet returns the tipset at the given key
func (chainInfoAPI *ChainInfoAPI) ChainGetTipSet(key types.TipSetKey) (*types.TipSet, error) {
	return chainInfoAPI.chain.ChainReader.GetTipSet(key)
}

// ChainGetTipSetByHeight looks back for a tipset at the specified epoch.
// If there are no blocks at the specified epoch, a tipset at an earlier epoch
// will be returned.
func (chainInfoAPI *ChainInfoAPI) ChainGetTipSetByHeight(ctx context.Context, height abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := chainInfoAPI.chain.ChainReader.GetTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("fail to load tipset %v", err)
	}
	return chainInfoAPI.chain.ChainReader.GetTipSetByHeight(ctx, ts, height, true)
}

// GetParentStateRootActor get the ts ParentStateRoot actor
func (chainInfoAPI *ChainInfoAPI) GetParentStateRootActor(ctx context.Context, ts *types.TipSet, addr address.Address) (*types.Actor, error) {
	if ts == nil {
		ts = chainInfoAPI.chain.ChainReader.GetHead()
	}
	v, err := chainInfoAPI.chain.ChainReader.ParentStateView(ts)
	if err != nil {
		return nil, err
	}
	act, err := v.LoadActor(ctx, addr)
	if err != nil {
		return nil, err
	}
	return act, nil
}

// ChainGetBlock gets a block by CID
func (chainInfoAPI *ChainInfoAPI) ChainGetBlock(ctx context.Context, id cid.Cid) (*types.BlockHeader, error) {
	return chainInfoAPI.chain.ChainReader.GetBlock(ctx, id)
}

// ChainGetMessage reads a message referenced by the specified CID from the
// chain blockstore.
func (chainInfoAPI *ChainInfoAPI) ChainGetMessage(ctx context.Context, msgID cid.Cid) (*types.UnsignedMessage, error) {
	msg, err := chainInfoAPI.chain.MessageStore.LoadMessage(msgID)
	if err != nil {
		return nil, err
	}
	return msg.VMMessage(), nil
}

// ChainGetMessages gets a message collection by CID
func (chainInfoAPI *ChainInfoAPI) ChainGetBlockMessages(ctx context.Context, bid cid.Cid) (*BlockMessages, error) {
	b, err := chainInfoAPI.chain.ChainReader.GetBlock(ctx, bid)
	if err != nil {
		return nil, err
	}

	smsgs, bmsgs, err := chainInfoAPI.chain.MessageStore.LoadMetaMessages(ctx, b.Messages)
	if err != nil {
		return nil, err
	}

	cids := make([]cid.Cid, len(bmsgs)+len(smsgs))

	for i, m := range bmsgs {
		cids[i] = m.Cid()
	}

	for i, m := range smsgs {
		cids[i+len(bmsgs)] = m.Cid()
	}

	return &BlockMessages{
		BlsMessages:   bmsgs,
		SecpkMessages: smsgs,
		Cids:          cids,
	}, nil
}

// ChainGetReceipts gets a receipt collection by CID
func (chainInfoAPI *ChainInfoAPI) ChainGetReceipts(ctx context.Context, bid cid.Cid) ([]types.MessageReceipt, error) {
	return chainInfoAPI.chain.MessageStore.LoadReceipts(ctx, bid)
}

// ChainGetFullBlock gets full block(include message) by cid
func (chainInfoAPI *ChainInfoAPI) ChainGetFullBlock(ctx context.Context, bid cid.Cid) (*types.FullBlock, error) {
	var out types.FullBlock
	var err error

	out.Header, err = chainInfoAPI.chain.ChainReader.GetBlock(ctx, bid)
	if err != nil {
		return nil, err
	}
	out.SECPMessages, out.BLSMessages, err = chainInfoAPI.chain.MessageStore.LoadMetaMessages(ctx, out.Header.Messages)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// ChainGetParentMessages returns messages stored in parent tipset of the
// specified block.
func (chainInfoAPI *ChainInfoAPI) ChainGetParentMessages(ctx context.Context, bcid cid.Cid) ([]Message, error) {
	b, err := chainInfoAPI.ChainGetBlock(ctx, bcid)
	if err != nil {
		return nil, err
	}

	// genesis block has no parent messages...
	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := chainInfoAPI.chain.ChainReader.GetTipSet(types.NewTipSetKey(b.Parents.Cids()...))
	if err != nil {
		return nil, err
	}

	cm, err := chainInfoAPI.chain.MessageStore.MessagesForTipset(pts)
	if err != nil {
		return nil, err
	}

	var out []Message
	for _, m := range cm {
		out = append(out, Message{
			Cid:     m.Cid(),
			Message: m.VMMessage(),
		})
	}

	return out, nil
}

// ChainGetParentReceipts returns receipts for messages in parent tipset of
// the specified block.
func (chainInfoAPI *ChainInfoAPI) ChainGetParentReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error) {
	b, err := chainInfoAPI.ChainGetBlock(ctx, bcid)
	if err != nil {
		return nil, err
	}

	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := chainInfoAPI.chain.ChainReader.GetTipSet(types.NewTipSetKey(b.Parents.Cids()...))
	if err != nil {
		return nil, err
	}

	cm, err := chainInfoAPI.chain.MessageStore.MessagesForTipset(pts)
	if err != nil {
		return nil, err
	}

	var out []*types.MessageReceipt
	for i := 0; i < len(cm); i++ {
		r, err := chainInfoAPI.chain.ChainReader.GetParentReceipt(b, i)
		if err != nil {
			return nil, err
		}

		out = append(out, r)
	}

	return out, nil
}

// ChainNotify subscribe to chain head change event
func (chainInfoAPI *ChainInfoAPI) ChainNotify(ctx context.Context) chan []*chain.HeadChange {
	return chainInfoAPI.chain.ChainReader.SubHeadChanges(ctx)
}

// StateNetworkName returns the name of the network the node is synced to
func (chainInfoAPI *ChainInfoAPI) StateNetworkName(ctx context.Context) (NetworkName, error) {
	networkName, err := chainInfoAPI.getNetworkName(ctx)

	return NetworkName(networkName), err
}

func (chainInfoAPI *ChainInfoAPI) getNetworkName(ctx context.Context) (string, error) {
	headKey := chainInfoAPI.chain.ChainReader.GetHead()
	view, err := chainInfoAPI.chain.ChainReader.ParentStateView(headKey)
	if err != nil {
		return "", err
	}

	return view.InitNetworkName(ctx)
}

// ChainGetRandomnessFromBeacon is used to sample the beacon for randomness.
func (chainInfoAPI *ChainInfoAPI) ChainGetRandomnessFromBeacon(ctx context.Context, key types.TipSetKey, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return chainInfoAPI.chain.ChainReader.ChainGetRandomnessFromBeacon(ctx, key, personalization, randEpoch, entropy)
}

// ChainGetRandomnessFromTickets is used to sample the chain for randomness.
func (chainInfoAPI *ChainInfoAPI) ChainGetRandomnessFromTickets(ctx context.Context, tsk types.TipSetKey, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	ts, err := chainInfoAPI.chain.ChainReader.GetTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset key: %v", err)
	}

	h := ts.Height()
	if randEpoch > h {
		return nil, xerrors.Errorf("cannot draw randomness from the future")
	}

	searchHeight := randEpoch
	if searchHeight < 0 {
		searchHeight = 0
	}

	randTs, err := chainInfoAPI.ChainGetTipSetByHeight(ctx, searchHeight, tsk)
	if err != nil {
		return nil, err
	}

	mtb := randTs.MinTicketBlock()

	return chain.DrawRandomness(mtb.Ticket.VRFProof, personalization, randEpoch, entropy)
}

// StateNetworkVersion returns the network version at the given tipset
func (chainInfoAPI *ChainInfoAPI) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	ts, err := chainInfoAPI.chain.ChainReader.GetTipSet(tsk)
	if err != nil {
		return network.VersionMax, xerrors.Errorf("loading tipset %s: %v", tsk, err)
	}
	return chainInfoAPI.chain.Fork.GetNtwkVersion(ctx, ts.Height()), nil
}

// MessageWait invokes the callback when a message with the given cid appears on chain.
// It will find the message in both the case that it is already on chain and
// the case that it appears in a newly mined block. An error is returned if one is
// encountered or if the context is canceled. Otherwise, it waits forever for the message
// to appear on chain.
func (chainInfoAPI *ChainInfoAPI) MessageWait(ctx context.Context, msgCid cid.Cid, confidence, lookback abi.ChainEpoch) (*chain.ChainMessage, error) {
	chainMsg, err := chainInfoAPI.chain.MessageStore.LoadMessage(msgCid)
	if err != nil {
		return nil, err
	}
	return chainInfoAPI.chain.Waiter.Wait(ctx, chainMsg, confidence, lookback)
}

// StateSearchMsg searches for a message in the chain, and returns its receipt and the tipset where it was executed
func (chainInfoAPI *ChainInfoAPI) StateSearchMsg(ctx context.Context, mCid cid.Cid) (*MsgLookup, error) {
	chainMsg, err := chainInfoAPI.chain.MessageStore.LoadMessage(mCid)
	if err != nil {
		return nil, err
	}
	//todo add a api for head tipset directly
	head := chainInfoAPI.chain.ChainReader.GetHead()
	msgResult, found, err := chainInfoAPI.chain.Waiter.Find(ctx, chainMsg, constants.LookbackNoLimit, head)
	if err != nil {
		return nil, err
	}

	if found {
		return &MsgLookup{
			Message: mCid,
			Receipt: *msgResult.Receipt,
			TipSet:  msgResult.Ts.Key(),
			Height:  msgResult.Ts.Height(),
		}, nil
	}
	return nil, nil
}

// StateWaitMsg looks back in the chain for a message. If not found, it blocks until the
// message arrives on chain, and gets to the indicated confidence depth.
func (chainInfoAPI *ChainInfoAPI) StateWaitMsg(ctx context.Context, mCid cid.Cid, confidence abi.ChainEpoch) (*MsgLookup, error) {
	chainMsg, err := chainInfoAPI.chain.MessageStore.LoadMessage(mCid)
	if err != nil {
		return nil, err
	}
	msgResult, err := chainInfoAPI.chain.Waiter.Wait(ctx, chainMsg, confidence, constants.LookbackNoLimit)
	if err != nil {
		return nil, err
	}
	if msgResult != nil {
		return &MsgLookup{
			Message: mCid,
			Receipt: *msgResult.Receipt,
			TipSet:  msgResult.Ts.Key(),
			Height:  msgResult.Ts.Height(),
		}, nil
	}
	return nil, nil
}

// StateGetReceipt returns the message receipt for the given message
func (chainInfoAPI *ChainInfoAPI) StateGetReceipt(ctx context.Context, msg cid.Cid, tsk types.TipSetKey) (*types.MessageReceipt, error) {
	chainMsg, err := chainInfoAPI.chain.MessageStore.LoadMessage(msg)
	if err != nil {
		return nil, err
	}
	//todo add a api for head tipset directly
	head := chainInfoAPI.chain.ChainReader.GetHead()

	msgResult, found, err := chainInfoAPI.chain.Waiter.Find(ctx, chainMsg, constants.LookbackNoLimit, head)
	if err != nil {
		return nil, err
	}

	if found {
		return msgResult.Receipt, nil
	}
	return nil, nil
}
