package chain

import (
	"context"
	"github.com/filecoin-project/venus_lite/app/client/apiface"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	xerrors "github.com/pkg/errors"
)

var _ apiface.IAccount = &accountAPI{}

type accountAPI struct {
	chain *ChainSubmodule
}

//NewAccountAPI create a new account api
func NewAccountAPI(chain *ChainSubmodule) apiface.IAccount {
	return &accountAPI{chain: chain}
}

// StateAccountKey returns the public key address of the given ID address
func (accountAPI *accountAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk cid.Cid) (address.Address, error) {
	ts, err := accountAPI.chain.ChainReader.GetBlock(ctx, tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %v", tsk, err)
	}

	view, err := accountAPI.chain.ChainReader.StateView(ts)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %v", tsk, err)
	}

	return view.ResolveToKeyAddr(ctx, addr)
}
