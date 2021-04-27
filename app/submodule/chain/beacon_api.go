package chain

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/pkg/types"
	xerrors "github.com/pkg/errors"
)

type IBeacon interface {
	BeaconGetEntry(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error)
	GetEntry(ctx context.Context, height abi.ChainEpoch, round uint64) (*types.BeaconEntry, error)
	VerifyEntry(parent, child *types.BeaconEntry, height abi.ChainEpoch) bool
}

var _ IBeacon = &BeaconAPI{}

type BeaconAPI struct {
	chain *ChainSubmodule
}

//NewBeaconAPI create new beacon api
func NewBeaconAPI(chain *ChainSubmodule) BeaconAPI {
	return BeaconAPI{chain: chain}
}

// BeaconGetEntry returns the beacon entry for the given filecoin epoch. If
// the entry has not yet been produced, the call will block until the entry
// becomes available
func (beaconAPI *BeaconAPI) BeaconGetEntry(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	b := beaconAPI.chain.Drand.BeaconForEpoch(epoch)
	rr := b.MaxBeaconRoundForEpoch(epoch)
	e := b.Entry(ctx, rr)

	select {
	case be, ok := <-e:
		if !ok {
			return nil, fmt.Errorf("beacon get returned no value")
		}
		if be.Err != nil {
			return nil, be.Err
		}
		return &be.Entry, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetEntry retrieves an entry from the drand server
func (beaconAPI *BeaconAPI) GetEntry(ctx context.Context, height abi.ChainEpoch, round uint64) (*types.BeaconEntry, error) {
	rch := beaconAPI.chain.Drand.BeaconForEpoch(height).Entry(ctx, round)
	select {
	case resp := <-rch:
		if resp.Err != nil {
			return nil, xerrors.Errorf("beacon entry request returned error: %s", resp.Err)
		}
		return &resp.Entry, nil
	case <-ctx.Done():
		return nil, xerrors.Errorf("context timed out waiting on beacon entry to come back for round %d: %s", round, ctx.Err())
	}
}

// VerifyEntry verifies that child is a valid entry if its parent is.
func (beaconAPI *BeaconAPI) VerifyEntry(parent, child *types.BeaconEntry, height abi.ChainEpoch) bool {
	return beaconAPI.chain.Drand.BeaconForEpoch(height).VerifyEntry(*parent, *child) != nil
}
