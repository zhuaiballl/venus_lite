package paych

import (
	"context"
	"github.com/filecoin-project/venus/pkg/paychmgr"
)

//PaychSubmodule 支持一系列和paych相关的功能，包括paych的构造，提取，查询等功能。
type PaychSubmodule struct { //nolint
	pmgr *paychmgr.Manager
}

// PaychSubmodule enhances the `Node` with paych capabilities.
func NewPaychSubmodule(ctx context.Context, params *paychmgr.ManagerParams) *PaychSubmodule {
	mgr := paychmgr.NewManager(ctx, params)
	return &PaychSubmodule{mgr}
}

func (ps *PaychSubmodule) Start() error {
	return ps.pmgr.Start()
}

func (ps *PaychSubmodule) Stop() {
	ps.pmgr.Stop()
}

//API create a new paych implement
func (ps *PaychSubmodule) API() IPaychan {
	return newPaychAPI(ps.pmgr)
}
