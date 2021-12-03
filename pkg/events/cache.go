package events

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus_lite/pkg/chain"
	"github.com/filecoin-project/venus_lite/pkg/types"
)

type uncachedAPI interface {
	ChainNotify(context.Context) <-chan []*chain.HeadChange
	ChainGetPath(ctx context.Context, from, to cid.Cid) ([]*chain.HeadChange, error)
	StateSearchMsg(ctx context.Context, from cid.Cid, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*chain.MsgLookup, error)

	StateGetActor(ctx context.Context, actor address.Address, tsk cid.Cid) (*types.Actor, error) // optional / for CalledMsg

	ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, cid.Cid) (*types.BlockHeader, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSet(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainHead(context.Context) (*types.BlockHeader, error)
}

type cache struct {
	//*tipSetCache
	*messageCache
	uncachedAPI
}

func newCache(api IEvent, gcConfidence abi.ChainEpoch) *cache {
	return &cache{
		//newTSCache(api, gcConfidence),
		newMessageCache(api),
		api,
	}
}
