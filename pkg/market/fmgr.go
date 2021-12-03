package market

import (
	"context"
	"github.com/filecoin-project/venus_lite/app/client/apiface"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/ipfs/go-cid"
)

// fundManagerAPI is the specific methods called by the FundManager
// (used by the tests)
type fundManager interface {
	MpoolPushMessage(context.Context, *types.UnsignedMessage, *types.MessageSendSpec) (*types.SignedMessage, error)
	StateMarketBalance(context.Context, address.Address, cid.Cid) (apitypes.MarketBalance, error)
	StateWaitMsg(ctx context.Context, c cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*apitypes.MsgLookup, error)
}

type fmgr struct {
	MPoolAPI      apiface.IMessagePool
	ChainInfoAPI  apiface.IChainInfo
	MinerStateAPI apiface.IMinerState
}

func newFundmanager(p *FundManagerParams) fundManager {
	fmAPI := &fmgr{
		MPoolAPI:      p.MP,
		ChainInfoAPI:  p.CI,
		MinerStateAPI: p.MS,
	}

	return fmAPI
}

func (o *fmgr) MpoolPushMessage(ctx context.Context, msg *types.UnsignedMessage, spec *types.MessageSendSpec) (*types.SignedMessage, error) {
	return o.MPoolAPI.MpoolPushMessage(ctx, msg, spec)
}

func (o *fmgr) StateMarketBalance(ctx context.Context, address address.Address, tsk cid.Cid) (apitypes.MarketBalance, error) {
	return o.MinerStateAPI.StateMarketBalance(ctx, address, tsk)
}

func (o *fmgr) StateWaitMsg(ctx context.Context, c cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*apitypes.MsgLookup, error) {
	return o.ChainInfoAPI.StateWaitMsg(ctx, c, confidence, limit, allowReplaced)
}
