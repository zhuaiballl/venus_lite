package chain

import (
	"context"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	lru "github.com/hashicorp/golang-lru"
	xerrors "github.com/pkg/errors"
)

//TODO: delete or modify all the structure and function using tipset
//TODO: try to achieve the same in the fileDAG according to this file

var DefaultChainIndexCacheSize = 32 << 10

//ChainIndex tipset height index, used to getting tipset by height quickly
type ChainIndex struct { //nolint
	skipCache *lru.ARCCache

	//loadTipSet loadTipSetFunc
	loadBlock loadBlockFunc

	skipLength abi.ChainEpoch
}

/*//NewChainIndex return a new chain index with arc cache
func NewChainIndex(lts loadTipSetFunc) *ChainIndex {
	sc, _ := lru.NewARC(DefaultChainIndexCacheSize)
	return &ChainIndex{
		skipCache:  sc,
		loadTipSet: lts,    //in chain/store.go GetTipSet this function will be changed
		skipLength: 20,
	}
}*/

//NewChainIndex return a new chain index with arc cache
func NewChainIndex(lb loadBlockFunc) *ChainIndex {
	sc, _ := lru.NewARC(DefaultChainIndexCacheSize)
	return &ChainIndex{
		skipCache: sc,
		//loadTipSet: lts,
		loadBlock:  lb, //in chain/store.go GetTipSet this function will be changed
		skipLength: 20,
	}
}

type lbEntry struct {
	ts           *types.BlockHeader
	parentHeight abi.ChainEpoch
	targetHeight abi.ChainEpoch
	target       cid.Cid
}

// GetTipSetByHeight get blockheader at specify height from specify blockheader
// the blockheader within the skiplength is directly obtained by reading the database.
// if the height difference exceeds the skiplength, the blockheader is read from caching.
// if the caching fails, the blockheader is obtained by reading the database and updating the cache
func (ci *ChainIndex) GetTipSetByHeight(_ context.Context, from *types.BlockHeader, to abi.ChainEpoch) (*types.BlockHeader, error) {
	if from.Height-to <= ci.skipLength {
		return ci.walkBack(from, to)
	}

	rounded, err := ci.roundDown(from)
	if err != nil {
		return nil, err
	}

	cur := rounded.Cid()
	// cur := from.Key()
	for {
		cval, ok := ci.skipCache.Get(cur)
		if !ok {
			fc, err := ci.fillCache(cur)
			if err != nil {
				return nil, err
			}
			cval = fc
		}

		lbe := cval.(*lbEntry)
		if lbe.ts.Height == to || lbe.parentHeight < to {
			return lbe.ts, nil
		} else if to > lbe.targetHeight {
			return ci.walkBack(lbe.ts, to)
		}
		//else if to < lbe.targetHeight ,need next for to change the target such as from 43 to 3 ,get the target less than 40,
		//so the height 40-20 is found,then the next for, the 20-0 is found
		cur = lbe.target
	}
}

//GetTipsetByHeightWithoutCache get the tipset of specific height by reading the database directly
func (ci *ChainIndex) GetTipsetByHeightWithoutCache(from *types.BlockHeader, to abi.ChainEpoch) (*types.BlockHeader, error) {
	return ci.walkBack(from, to)
}

//so,for simple,can we just use reading the database??

func (ci *ChainIndex) fillCache(tsk cid.Cid) (*lbEntry, error) {
	ts, err := ci.loadBlock(nil, tsk)
	if err != nil {
		return nil, err
	}

	if ts.Height == 0 {
		return &lbEntry{
			ts:           ts,
			parentHeight: 0,
		}, nil
	}

	// will either be equal to ts.Height, or at least > ts.Parent.Height()
	rheight := ci.roundHeight(ts.Height)

	parent, err := ci.loadBlock(nil, ts.Parent)
	if err != nil {
		return nil, err
	}

	rheight -= ci.skipLength
	if rheight < 0 {
		rheight = 0
	}

	var skipTarget *types.BlockHeader
	if parent.Height < rheight {
		skipTarget = parent
	} else {
		skipTarget, err = ci.walkBack(parent, rheight)
		if err != nil {
			return nil, xerrors.Errorf("fillCache walkback: %s", err)
		}
	}

	lbe := &lbEntry{
		ts:           ts,
		parentHeight: parent.Height,
		targetHeight: skipTarget.Height,
		target:       skipTarget.Cid(),
	}
	ci.skipCache.Add(tsk, lbe)

	return lbe, nil
}

// floors to nearest skipLength multiple
func (ci *ChainIndex) roundHeight(h abi.ChainEpoch) abi.ChainEpoch {
	return (h / ci.skipLength) * ci.skipLength
}

func (ci *ChainIndex) roundDown(ts *types.BlockHeader) (*types.BlockHeader, error) {
	target := ci.roundHeight(ts.Height)

	rounded, err := ci.walkBack(ts, target)
	if err != nil {
		return nil, err
	}

	return rounded, nil
}

func (ci *ChainIndex) walkBack(from *types.BlockHeader, to abi.ChainEpoch) (*types.BlockHeader, error) {
	if to > from.Height {
		return nil, xerrors.Errorf("looking for tipset with height greater than start point")
	}

	if to == from.Height {
		return from, nil
	}

	ts := from

	for {
		pts, err := ci.loadBlock(nil, ts.Parent)
		if err != nil {
			return nil, err
		}

		if to > pts.Height {
			// in case pts is lower than the epoch we're looking for (null blocks)
			// return a tipset above that height
			return ts, nil
		}
		if to == pts.Height {
			return pts, nil
		}

		ts = pts
	}
}
