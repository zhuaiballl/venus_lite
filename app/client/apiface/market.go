package apiface

import (
	"context"

	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	"github.com/filecoin-project/venus_lite/pkg/types"
)

type IMarket interface {
	// Rule[perm:read]
	StateMarketParticipants(ctx context.Context, tsk types.TipSetKey) (map[string]apitypes.MarketBalance, error) //perm:admin
}
