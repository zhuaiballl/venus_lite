package chain

import (
	"context"
	"errors"
	"github.com/filecoin-project/venus_lite/pkg/types"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
)

// TipSetProvider provides tipsets for traversal.
type TipSetProvider interface {
	GetTipSet(types.TipSetKey) (*types.TipSet, error)
	GetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
}

// IterAncestors returns an iterator over tipset ancestors, yielding first the start tipset and
// then its parent tipsets until (and including) the genesis tipset.
func IterAncestors(ctx context.Context, store TipSetProvider, start *types.BlockHeader) *TipsetIterator {
	return &TipsetIterator{ctx, store, start}
}

// TipsetIterator is an iterator over tipsets.
type TipsetIterator struct {
	ctx   context.Context
	store TipSetProvider
	value *types.BlockHeader
}

// Value returns the iterator's current value, if not Complete().
func (it *TipsetIterator) Value() *types.BlockHeader {
	return it.value
}

// Complete tests whether the iterator is exhausted.
func (it *TipsetIterator) Complete() bool {
	return !(it.value == nil)
}

// Next advances the iterator to the next value.
func (it *TipsetIterator) Next() error {
	select {
	case <-it.ctx.Done():
		return it.ctx.Err()
	default:
		if it.value.Height == 0 {
			it.value = &types.BlockHeader{}
		} else {
			var err error
			parentKey := it.value.Parent
			it.value, err = it.store.GetBlock(it.ctx, parentKey)
			return err
		}
		return nil
	}
}

// BlockProvider provides blocks.
type BlockProvider interface {
	GetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
}

// LoadTipSetBlocks loads all the blocks for a tipset from the store.
func LoadTipSetBlocks(ctx context.Context, store BlockProvider, key types.TipSetKey) (*types.TipSet, error) {
	var blocks []*types.BlockHeader
	for _, bid := range key.Cids() {
		blk, err := store.GetBlock(ctx, bid)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, blk)
	}
	return types.NewTipSet(blocks...)
}

type tipsetFromBlockProvider struct {
	ctx    context.Context // Context to use when loading blocks
	blocks BlockProvider   // Provides blocks
}

// TipSetProviderFromBlocks builds a tipset provider backed by a block provider.
// Blocks will be loaded with the provided context, since GetTipSet does not accept a
// context parameter. This can and should be removed when GetTipSet does take a context.
//TODO:this function is not used!
func TipSetProviderFromBlocks(ctx context.Context, blocks BlockProvider) TipSetProvider {
	return &tipsetFromBlockProvider{ctx, blocks}
}

// GetTipSet loads the blocks for a tipset.
func (p *tipsetFromBlockProvider) GetTipSet(tsKey types.TipSetKey) (*types.TipSet, error) {
	return LoadTipSetBlocks(p.ctx, p.blocks, tsKey)
}

func (p *tipsetFromBlockProvider) GetBlock(ctx context.Context, c cid.Cid) (*types.BlockHeader, error) {
	panic("implement me")
}

// CollectTipsToCommonAncestor traverses chains from two tipsets (called old and new) until their common
// ancestor, collecting all tipsets that are in one chain but not the other.
// The resulting lists of tipsets are ordered by decreasing height.
func CollectTipsToCommonAncestor(ctx context.Context, store TipSetProvider, oldHead, newHead *types.BlockHeader) (oldTips, newTips []*types.BlockHeader, err error) {
	oldIter := IterAncestors(ctx, store, oldHead)
	newIter := IterAncestors(ctx, store, newHead)

	commonAncestor, err := FindCommonAncestor(oldIter, newIter)
	if err != nil {
		return
	}
	commonHeight := commonAncestor.Height

	// Refresh iterators modified by FindCommonAncestors
	oldIter = IterAncestors(ctx, store, oldHead)
	newIter = IterAncestors(ctx, store, newHead)

	// Add 1 to the height argument so that the common ancestor is not
	// included in the outputs.
	oldTips, err = CollectTipSetsOfHeightAtLeast(ctx, oldIter, commonHeight+1)
	if err != nil {
		return
	}
	newTips, err = CollectTipSetsOfHeightAtLeast(ctx, newIter, commonHeight+1)
	return
}

// ErrNoCommonAncestor is returned when two chains assumed to have a common ancestor do not.
var ErrNoCommonAncestor = errors.New("no common ancestor")

// FindCommonAncestor returns the common ancestor of the two tipsets pointed to
// by the input iterators.  If they share no common ancestor ErrNoCommonAncestor
// will be returned.
func FindCommonAncestor(leftIter, rightIter *TipsetIterator) (*types.BlockHeader, error) {
	for !rightIter.Complete() && !leftIter.Complete() {
		left := leftIter.Value()
		right := rightIter.Value()

		leftHeight := left.Height
		rightHeight := right.Height

		// Found common ancestor.
		if left.Equals(right) {
			return left, nil
		}

		// Update the pointers.  Pointers move back one tipset if they
		// point to a tipset at the same height or higher than the
		// other pointer's tipset.
		if rightHeight >= leftHeight {
			if err := rightIter.Next(); err != nil {
				return nil, err
			}
		}

		if leftHeight >= rightHeight {
			if err := leftIter.Next(); err != nil {
				return nil, err
			}
		}
	}
	return nil, ErrNoCommonAncestor
}

// CollectTipSetsOfHeightAtLeast collects all tipsets with a height greater
// than or equal to minHeight from the input tipset.
func CollectTipSetsOfHeightAtLeast(ctx context.Context, iterator *TipsetIterator, minHeight abi.ChainEpoch) ([]*types.BlockHeader, error) {
	var ret []*types.BlockHeader
	var err error
	var h abi.ChainEpoch
	for ; !iterator.Complete(); err = iterator.Next() {
		if err != nil {
			return nil, err
		}
		h = iterator.Value().Height
		if h < minHeight {
			return ret, nil
		}
		ret = append(ret, iterator.Value())
	}
	return ret, nil
}

// FindLatestDRAND returns the latest DRAND entry in the chain beginning at start
func FindLatestDRAND(ctx context.Context, start *types.BlockHeader, reader TipSetProvider) (*types.BeaconEntry, error) {
	iterator := IterAncestors(ctx, reader, start)
	var err error
	for ; !iterator.Complete(); err = iterator.Next() {
		if err != nil {
			return nil, err
		}
		ts := iterator.Value()
		// DRAND entries must be the same for all blocks on the tipset as
		// an invariant of the tipset provider

		entries := ts.BeaconEntries
		if len(entries) > 0 {
			return entries[len(entries)-1], nil
		}
		// No entries, simply move on to the next ancestor
	}
	return nil, errors.New("no DRAND entries in chain")
}
