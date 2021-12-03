package events

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	"github.com/filecoin-project/venus_lite/pkg/chain"
	"github.com/filecoin-project/venus_lite/pkg/types"
)

// A TipSetObserver receives notifications of tipsets
type TipSetObserver interface {
	Apply(ctx context.Context, from, to *types.BlockHeader) error
	Revert(ctx context.Context, from, to *types.BlockHeader) error
}

type IEvent interface {
	ChainNotify(context.Context) <-chan []*chain.HeadChange
	ChainGetBlockMessages(context.Context, cid.Cid) (*apitypes.BlockMessages, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, cid.Cid) (*types.BlockHeader, error)
	ChainHead(context.Context) (*types.BlockHeader, error)
	StateSearchMsg(ctx context.Context, from cid.Cid, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*apitypes.MsgLookup, error)
	ChainGetTipSet(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainGetPath(ctx context.Context, from, to cid.Cid) ([]*chain.HeadChange, error)

	StateGetActor(ctx context.Context, actor address.Address, tsk cid.Cid) (*types.Actor, error) // optional / for CalledMsg
}
