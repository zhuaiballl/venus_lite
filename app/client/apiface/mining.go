package apiface

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	"github.com/filecoin-project/venus_lite/pkg/types"
)

type IMining interface {
	// Rule[perm:read]
	MinerGetBaseInfo(ctx context.Context, maddr address.Address, round abi.ChainEpoch, tsk types.TipSetKey) (*apitypes.MiningBaseInfo, error)
	// Rule[perm:read]
	MinerCreateBlock(ctx context.Context, bt *apitypes.BlockTemplate) (*types.BlockMsg, error)
}
