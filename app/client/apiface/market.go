package apiface

import (
	"context"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
)

type IMarket interface {
	// Rule[perm:read]
	StateMarketParticipants(ctx context.Context, tsk cid.Cid) (map[string]apitypes.MarketBalance, error) //perm:admin
}
