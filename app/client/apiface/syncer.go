package apiface

import (
	"context"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	syncTypes "github.com/filecoin-project/venus_lite/pkg/chainsync/types"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/ipfs/go-cid"
)

type ISyncer interface {
	// Rule[perm:read]
	ChainSyncHandleNewTipSet(ctx context.Context, ci *types.ChainInfo) error
	// Rule[perm:read]
	SetConcurrent(ctx context.Context, concurrent int64) error
	// Rule[perm:read]
	SyncerTracker(ctx context.Context) *syncTypes.TargetTracker
	// Rule[perm:read]
	Concurrent(ctx context.Context) int64
	// Rule[perm:read]
	ChainTipSetWeight(ctx context.Context, tsk cid.Cid) (big.Int, error)
	// Rule[perm:read]
	SyncSubmitBlock(ctx context.Context, blk *types.BlockMsg) error
	// Rule[perm:read]
	StateCall(ctx context.Context, msg *types.UnsignedMessage, tsk cid.Cid) (*types.InvocResult, error)
	// Rule[perm:read]
	SyncState(ctx context.Context) (*apitypes.SyncState, error)
}
