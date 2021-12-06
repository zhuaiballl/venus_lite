package chain

import (
	"bytes"
	"context"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/filecoin-project/venus_lite/pkg/constants"
	"github.com/filecoin-project/venus_lite/pkg/state"
	"github.com/filecoin-project/venus_lite/pkg/state/tree"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin"
	_init "github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/init"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/market"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/miner"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/multisig"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/power"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/reward"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/verifreg"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/policy"
	"github.com/filecoin-project/venus_lite/pkg/util"
	"github.com/ipld/go-car"
	carutil "github.com/ipld/go-car/util"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
	"io"
	"os"
	"runtime/debug"
	"sync"

	"github.com/filecoin-project/venus_lite/pkg/repo"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/adt"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/pubsub"
)

// HeadChangeTopic is the topic used to publish new heads.
const (
	HeadChangeTopic = "headchange"
	HCRevert        = "revert"
	HCApply         = "apply"
	HCCurrent       = "current"
)

// ErrNoMethod is returned by Get when there is no method signature (eg, transfer).
var ErrNoMethod = errors.New("no method")

// ErrNoActorImpl is returned by Get when the actor implementation doesn't exist, eg
// the actor address is an empty actor, an address that has received a transfer of FIL
// but hasn't yet been upgraded to an account actor. (The actor implementation might
// also genuinely be missing, which is not expected.)
var ErrNoActorImpl = errors.New("no actor implementation")

// GenesisKey is the key at which the genesis Cid is written in the datastore.
var GenesisKey = datastore.NewKey("/consensus/genesisCid")

var log = logging.Logger("chain.store")

// HeadKey is the key at which the head tipset cid's are written in the datastore.
var HeadKey = datastore.NewKey("/chain/heaviestTipSet")

var ErrNotifeeDone = errors.New("notifee is done and should be removed")

type loadTipSetFunc func(types.TipSetKey) (*types.TipSet, error)

type loadBlockFunc func(ctx context.Context, blockID cid.Cid) (*types.BlockHeader, error)

// ReorgNotifee represents a callback that gets called upon reorgs.
type ReorgNotifee func(rev, app *types.BlockHeader) error

type reorg struct {
	old *types.BlockHeader
	new *types.BlockHeader
}

type HeadChange struct {
	Type string
	Val  *types.BlockHeader
}

// CheckPoint is the key which the check-point written in the datastore.
var CheckPoint = datastore.NewKey("/chain/checkPoint")

//TSState export this func is just for gen cbor tool to work
type TSState struct {
	StateRoot cid.Cid
	Receipts  cid.Cid
}

func ActorStore(ctx context.Context, bs blockstore.Blockstore) adt.Store {
	return adt.WrapStore(ctx, cbor.NewCborStore(bs))
}

// Store is a generic implementation of the Store interface.
// It works(tm) for now.
type Store struct {
	// ipldSource is a wrapper around ipld storage.  It is used
	// for reading filecoin block and state objects kept by the node.
	stateAndBlockSource cbor.IpldStore

	bsstore blockstore.Blockstore

	// ds is the datastore for the chain's private metadata which consists
	// of the tipset key to state root cid mapping, and the heaviest tipset
	// key.
	ds repo.Datastore

	// genesis is the CID of the genesis block.
	genesis cid.Cid
	// head is the tipset at the head of the best known chain.
	//head *types.TipSet
	head *types.BlockHeader

	//checkPoint types.TipSetKey
	// Protects head and genesisCid.
	checkPoint cid.Cid

	mu sync.RWMutex

	// headEvents is a pubsub channel that publishes an event every time the head changes.
	// We operate under the assumption that tipsets published to this channel
	// will always be queued and delivered to subscribers in the order discovered.
	// Successive published tipsets may be supersets of previously published tipsets.
	// TODO: rename to notifications.  Also, reconsider ordering assumption depending
	// on decisions made around the FC node notification system.
	// TODO: replace this with a synchronous event bus
	// https://github.com/filecoin-project/venus_lite/issues/2309
	headEvents *pubsub.PubSub

	// Tracks tipsets by height/parentset for use by expected consensus.
	//tipIndex *TipStateCache

	circulatingSupplyCalculator ICirculatingSupplyCalcualtor

	chainIndex *ChainIndex

	reorgCh        chan reorg
	reorgNotifeeCh chan ReorgNotifee

	tsCache *lru.ARCCache
}

// NewStore constructs a new default store.
func NewStore(chainDs repo.Datastore,
	bsstore blockstore.Blockstore,
	genesisCid cid.Cid,
	circulatiingSupplyCalculator ICirculatingSupplyCalcualtor,
) *Store {
	tsCache, _ := lru.NewARC(10000)
	store := &Store{
		stateAndBlockSource: cbor.NewCborStore(bsstore),
		ds:                  chainDs,
		bsstore:             bsstore,
		headEvents:          pubsub.New(64),

		//checkPoint:     types.UndefTipSet.Key(),
		genesis:        genesisCid,
		reorgNotifeeCh: make(chan ReorgNotifee),
		tsCache:        tsCache,
	}
	//todo cycle reference , may think a better idea
	//store.tipIndex = NewTipStateCache(store)
	//store.chainIndex = NewChainIndex(store.GetTipSet)
	store.chainIndex = NewChainIndex(store.GetBlock)
	store.circulatingSupplyCalculator = circulatiingSupplyCalculator

	val, err := store.ds.Get(CheckPoint)
	if err != nil {
		//store.checkPoint = types.NewTipSetKey(genesisCid)
		store.checkPoint = genesisCid
	} else {
		//_ = store.checkPoint.UnmarshalCBOR(bytes.NewReader(val)) //nolint:staticcheck
		_ = store.checkPoint.UnmarshalText(val)
	}
	log.Infof("check point value: %v", store.checkPoint.String())

	store.reorgCh = store.reorgWorker(context.TODO())
	return store
}

func (store *Store) Blockstore() blockstore.Blockstore { // nolint
	return store.bsstore
}

//load the blockchain from the known head from disk
func (store *Store) Load(ctx context.Context) (err error) {
	headBlockCid, err := store.loadHead()
	if err != nil {
		return err
	}

	headBH, err := store.GetBlock(ctx, headBlockCid)
	if err != nil {
		//return errors.Warp(err,"error loading head blockheader")
		return err
	}
	log.Infof("start loading chain at blockheader: %s,height: %d", headBH.Cid().String(), headBH.Height)
	crt := headBH
	for crt.Cid() != store.genesis {
		fmt.Println("get block, the cid is:%s", crt.Cid().String())
		crt, err := store.GetBlock(ctx, crt.Parent)
		if err != nil {
			return err
		}
		fmt.Println("next block:%s", crt.Cid().String())
	}
	log.Infof("finished loading chain from %s", headBH.Cid().String())
	return nil
}

// loadHead loads the latest known head from disk.
func (store *Store) loadHead() (cid.Cid, error) {
	var emptyCid cid.Cid
	tskBytes, err := store.ds.Get(HeadKey) //"/chain/heaviestTipSet",need to be changed??
	if err != nil {
		return emptyCid, errors.Wrap(err, "failed to read HeadKey")
	}

	var tsk cid.Cid
	err = tsk.UnmarshalText(tskBytes)
	if err != nil {
		return emptyCid, errors.Wrap(err, "failed to cast headCid")
	}

	return tsk, nil
}

// GetBlock returns the block identified by `cid`.
func (store *Store) GetBlock(ctx context.Context, blockID cid.Cid) (*types.BlockHeader, error) {
	var block types.BlockHeader
	err := store.stateAndBlockSource.Get(ctx, blockID, &block)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block %s", blockID.String())
	}
	return &block, nil
}

func (store *Store) SubscribeHeadChanges(f ReorgNotifee) {
	store.reorgNotifeeCh <- f
}

// SetHead sets the passed in tipset as the new head of this chain.
func (store *Store) SetHead(ctx context.Context, newBH *types.BlockHeader) error {
	log.Infof("SetHead %s %d", newBH.String(), newBH.Height)
	// Add logging to debug sporadic test failure.
	if newBH == nil {
		log.Errorf("publishing empty blockheader")
		log.Error(debug.Stack())
		return nil
	}
	if newBH.Height > store.head.Height {
		log.Errorf("new head is older than the current head")
		log.Error(debug.Stack())
		return nil
	}
	if errInner := store.writeHead(ctx, newBH.Cid()); errInner != nil {
		return errInner
	}
	oldhead := store.head
	store.head = newBH
	store.reorgCh <- reorg{
		old: oldhead,
		new: newBH,
	}
	return nil
}

// writeHead writes the given cid set as head to disk.
func (store *Store) writeHead(ctx context.Context, blockID cid.Cid) error {
	log.Debugf("WriteHead %s", blockID.String())
	buf := blockID.Bytes()

	return store.ds.Put(HeadKey, buf)
}

// GetHead returns the current head tipset cids.
func (store *Store) GetHead() *types.BlockHeader {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.head == nil {
		return nil
	}

	return store.head
}

// GetGenesisBlock returns the genesis block held by the chain store.
func (store *Store) GetGenesisBlock(ctx context.Context) (*types.BlockHeader, error) {
	return store.GetBlock(ctx, store.genesis)
}

func (store *Store) GetGenesisCid() cid.Cid {
	return store.genesis
}

func (store *Store) PutObject(ctx context.Context, obj interface{}) (cid.Cid, error) {
	return store.stateAndBlockSource.Put(ctx, obj)
}

// PutMessage put message in local db
func (store *Store) PutMessage(m storable) (cid.Cid, error) {
	return PutMessage(store.bsstore, m)
}

// ReadOnlyStateStore provides a read-only IPLD store for access to chain state.
func (store *Store) ReadOnlyStateStore() util.ReadOnlyIpldStore {
	return util.ReadOnlyIpldStore{IpldStore: store.stateAndBlockSource}
}

func (store *Store) GetCheckPoint() cid.Cid {
	return store.checkPoint
}

func (store *Store) Stop() {
	store.headEvents.Shutdown()
}

//The Tipset in venus_lite is blockheader
func (store *Store) GetTipSetState(ctx context.Context, bh *types.BlockHeader) (tree.Tree, error) {
	if bh == nil {
		bh = store.head
	}
	stateCid := bh.Parent
	return tree.LoadState(ctx, store.stateAndBlockSource, stateCid)
}

// ResolveToKeyAddr get key address of specify address.
//if ths addr is bls/secpk address, return directly, other get the pubkey and generate address
func (store *Store) ResolveToKeyAddr(ctx context.Context, ts *types.BlockHeader, addr address.Address) (address.Address, error) {
	st, err := store.StateView(ts)
	if err != nil {
		return address.Undef, errors.Wrap(err, "failed to load latest state")
	}

	return st.ResolveToKeyAddr(ctx, addr)
}

// StateView return state view at ts epoch
func (store *Store) StateView(ts *types.BlockHeader) (*state.View, error) {
	if ts == nil {
		ts = store.head
	}
	root, err := store.GetTipSetStateRoot(ts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get state root for %s", ts.Cid().String())
	}

	return state.NewView(store.stateAndBlockSource, root), nil
}

// ParentStateView get parent state view of ts
func (store *Store) ParentStateView(ts *types.BlockHeader) (*state.View, error) {
	return state.NewView(store.stateAndBlockSource, ts.ParentStateRoot), nil
}

func (store *Store) reorgWorker(ctx context.Context) chan reorg {
	headChangeNotifee := func(rev, app *types.BlockHeader) error {
		notif := make([]*HeadChange, 2)
		notif[0] = &HeadChange{
			Type: HCRevert,
			Val:  app,
		}

		notif[1] = &HeadChange{
			Type: HCApply,
			Val:  rev,
		}
		// Publish an event that we have a new head.
		store.headEvents.Pub(notif, HeadChangeTopic)
		return nil
	}

	out := make(chan reorg, 32)
	notifees := []ReorgNotifee{headChangeNotifee}

	go func() {
		defer log.Warn("reorgWorker quit")
		for {
			select {
			case n := <-store.reorgNotifeeCh:
				notifees = append(notifees, n)

			case r := <-out:
				var toremove map[int]struct{}
				for i, hcf := range notifees {
					err := hcf(r.old, r.new)

					switch err {
					case nil:

					case ErrNotifeeDone:
						if toremove == nil {
							toremove = make(map[int]struct{})
						}
						toremove[i] = struct{}{}

					default:
						log.Error("head change func errored (BAD): ", err)
					}
				}

				if len(toremove) > 0 {
					newNotifees := make([]ReorgNotifee, 0, len(notifees)-len(toremove))
					for i, hcf := range notifees {
						_, remove := toremove[i]
						if remove {
							continue
						}
						newNotifees = append(newNotifees, hcf)
					}
					notifees = newNotifees
				}

			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func ReorgOps(lts func(context.Context, cid.Cid) (*types.BlockHeader, error), a, b *types.BlockHeader) ([]*types.BlockHeader, []*types.BlockHeader, error) {
	left := a
	right := b

	var leftChain, rightChain []*types.BlockHeader
	for !left.Equals(right) {
		lh := left.Height
		rh := right.Height
		if lh > rh {
			leftChain = append(leftChain, left)
			lKey := left.Parent
			par, err := lts(nil, lKey)
			if err != nil {
				return nil, nil, err
			}

			left = par
		} else {
			rightChain = append(rightChain, right)
			rKey := right.Parent
			par, err := lts(nil, rKey)
			if err != nil {
				log.Infof("failed to fetch right.Parents: %s", err)
				return nil, nil, err
			}

			right = par
		}
	}

	return leftChain, rightChain, nil

}

func (store *Store) GetTipSet(key types.TipSetKey) (*types.TipSet, error) {
	panic("implement me")
}

func (store *Store) GetTipSetStateRoot(ts *types.BlockHeader) (cid.Cid, error) {
	return ts.Parent, nil
}

func (store *Store) GetTipSetReceiptsRoot(ts *types.BlockHeader) (cid.Cid, error) {
	return ts.Messages, nil
}

func (store *Store) GetCirculatingSupplyDetailed(ctx context.Context, height abi.ChainEpoch, st tree.Tree) (CirculatingSupply, error) {
	return store.circulatingSupplyCalculator.GetCirculatingSupplyDetailed(ctx, height, st)
}

func (store *Store) AccountView(ts *types.BlockHeader) (state.AccountView, error) {
	if ts == nil {
		ts = store.head
	}
	root := ts.Parent
	return state.NewView(store.stateAndBlockSource, root), nil
}

func (store *Store) LookupID(ctx context.Context, ts *types.BlockHeader, addr address.Address) (address.Address, error) {
	st, err := store.GetTipSetState(ctx, ts)
	if err != nil {
		return address.Undef, errors.Wrap(err, "failed to load latest blockheader state")
	}
	return st.LookupID(addr)
}

func (store *Store) GetActorAt(ctx context.Context, ts *types.BlockHeader, addr address.Address) (*types.Actor, error) {
	st, err := store.GetTipSetState(ctx, ts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load latest blockheader state")
	}

	idAddr, err := store.LookupID(ctx, ts, addr)
	if err != nil {
		return nil, err
	}

	actr, found, err := st.GetActor(ctx, idAddr)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, types.ErrActorNotFound
	}
	return actr, nil
}

// GetTipSetByHeight looks back for a tipset at the specified epoch.
// If there are no blocks at the specified epoch, a tipset at an earlier epoch
// will be returned.

func (store *Store) GetTipSetByHeight(ctx context.Context, ts *types.BlockHeader, h abi.ChainEpoch, prev bool) (*types.BlockHeader, error) {
	if ts == nil {
		ts = store.head
	}

	if h > ts.Height {
		return nil, xerrors.Errorf("looking for tipset with height greater than start point")
	}

	if h == ts.Height {
		return ts, nil
	}

	lbts, err := store.chainIndex.GetTipSetByHeight(ctx, ts, h)
	if err != nil {
		return nil, err
	}

	if lbts.Height < h {
		log.Warnf("chain index returned the wrong tipset at height %d, using slow retrieval", h)
		lbts, err = store.chainIndex.GetTipsetByHeightWithoutCache(ts, h)
		if err != nil {
			return nil, err
		}
	}

	if lbts.Height == h || !prev {
		return lbts, nil
	}

	return store.GetBlock(ctx, lbts.Parent)

	/*lbts := ts
	for {
		if h >= lbts.Height {
			break
		}
		lbts, _ = store.GetBlock(ctx, lbts.Parent)
		if lbts == nil {
			return nil, xerrors.Errorf("get blockheader failed!")
		}
	}
	if h == lbts.Height {
		return lbts, nil
	}
	//else h>lbts.Height
	log.Warnf("in chain this is impossible.")
	return lbts, nil*/
}

//GetLatestBeaconEntry get latest beacon from the height. there're no beacon values in the block, try to
//get beacon in the parents tipset. the max find depth is 20.
func (store *Store) GetLatestBeaconEntry(ts *types.BlockHeader) (*types.BeaconEntry, error) {
	cur := ts
	for i := 0; i < 20; i++ {
		cbe := cur.BeaconEntries
		if len(cbe) > 0 {
			return cbe[len(cbe)-1], nil
		}

		if cur.Height == 0 {
			return nil, xerrors.Errorf("made it back to genesis block without finding beacon entry")
		}

		next, err := store.GetBlock(nil, cur.Parent)
		if err != nil {
			return nil, xerrors.Errorf("failed to load parents when searching back for latest beacon entry: %w", err)
		}
		cur = next
	}

	if os.Getenv("VENUS_IGNORE_DRAND") == "_yes_" {
		return &types.BeaconEntry{
			Data: []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
		}, nil
	}

	return nil, xerrors.Errorf("found NO beacon entries in the 20 blocks prior to given tipset")
}

// SubHeadChanges returns channel with chain head updates.
// First message is guaranteed to be of len == 1, and type == 'current'.
// Then event in the message may be HCApply and HCRevert.
func (store *Store) SubHeadChanges(ctx context.Context) chan []*HeadChange {
	out := make(chan []*HeadChange, 16)
	store.mu.RLock()
	head := store.head
	store.mu.RUnlock()
	out <- []*HeadChange{{
		Type: HCCurrent,
		Val:  head,
	}}

	subCh := store.headEvents.Sub(HeadChangeTopic)
	go func() {
		defer close(out)
		var unsubOnce sync.Once

		for {
			select {
			case val, ok := <-subCh:
				if !ok {
					log.Warn("chain head sub exit loop")
					return
				}
				if len(out) > 5 {
					log.Warnf("head change sub is slow, has %d buffered entries", len(out))
				}
				select {
				case out <- val.([]*HeadChange):
				case <-ctx.Done():
				}
			case <-ctx.Done():
				unsubOnce.Do(func() {
					go store.headEvents.Unsub(subCh)
				})
			}
		}
	}()
	return out
}

// GetLookbackTipSetForRound get loop back tipset and state root
func (store *Store) GetLookbackTipSetForRound(ctx context.Context, ts *types.BlockHeader, round abi.ChainEpoch, version network.Version) (*types.BlockHeader, cid.Cid, error) {
	var lbr abi.ChainEpoch

	lb := policy.GetWinningPoStSectorSetLookback(version)
	if round > lb {
		lbr = round - lb
	}

	// more null blocks than our lookback
	h := ts.Height
	if lbr >= h {
		// This should never happen at this point, but may happen before
		// network version 3 (where the lookback was only 10 blocks).
		st, err := store.GetTipSetStateRoot(ts)
		if err != nil {
			return nil, cid.Undef, err
		}
		return ts, st, nil
	}

	// Get the tipset after the lookback tipset, or the next non-null one.
	nextTS, err := store.GetTipSetByHeight(ctx, ts, lbr+1, false)
	if err != nil {
		return nil, cid.Undef, xerrors.Errorf("failed to get lookback blockheader+1: %v", err)
	}

	nextTh := nextTS.Height
	if lbr > nextTh {
		return nil, cid.Undef, xerrors.Errorf("failed to find non-null blockheader %s (%d) which is known to exist, found %s (%d)", ts.Cid(), h, nextTS.Cid(), nextTh)
	}

	pKey := nextTS.Parent
	lbts, err := store.GetBlock(ctx, pKey)
	if err != nil {
		return nil, cid.Undef, xerrors.Errorf("failed to resolve lookback tipset: %v", err)
	}

	return lbts, nextTS.ParentStateRoot, nil
}

// LsActors returns a channel with actors from the latest state on the chain
func (store *Store) LsActors(ctx context.Context) (map[address.Address]*types.Actor, error) {
	st, err := store.GetTipSetState(ctx, store.head)
	if err != nil {
		return nil, err
	}

	result := make(map[address.Address]*types.Actor)
	err = st.ForEach(func(key address.Address, a *types.Actor) error {
		result[key] = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Ls returns an iterator over tipsets from head to genesis.
func (store *Store) Ls(ctx context.Context, fromTS *types.BlockHeader, count int) ([]*types.BlockHeader, error) {
	tipsets := []*types.BlockHeader{fromTS}
	fromKey := fromTS.Parent
	for i := 0; i < count-1; i++ {
		ts, err := store.GetBlock(ctx, fromKey)
		if err != nil {
			return nil, err
		}
		tipsets = append(tipsets, ts)
		fromKey = ts.Parent
	}
	types.ReverseBlockHeader(tipsets)
	return tipsets, nil
}

// GetParentReceipt get the receipt of parent tipset at specify message slot
func (store *Store) GetParentReceipt(b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	ctx := context.TODO()
	// block headers use adt0, for now.
	a, err := blockadt.AsArray(adt.WrapStore(ctx, store.stateAndBlockSource), b.ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var r types.MessageReceipt
	if found, err := a.Get(uint64(i), &r); err != nil {
		return nil, err
	} else if !found {
		return nil, xerrors.Errorf("failed to find receipt %d", i)
	}

	return &r, nil
}

func recurseLinks(bs blockstore.Blockstore, walked *cid.Set, root cid.Cid, in []cid.Cid) ([]cid.Cid, error) {
	if root.Prefix().Codec != cid.DagCBOR {
		return in, nil
	}

	data, err := bs.Get(root)
	if err != nil {
		return nil, xerrors.Errorf("recurse links get (%s) failed: %w", root, err)
	}

	var rerr error
	err = cbg.ScanForLinks(bytes.NewReader(data.RawData()), func(c cid.Cid) {
		if rerr != nil {
			// No error return on ScanForLinks :(
			return
		}

		// traversed this already...
		if !walked.Visit(c) {
			return
		}

		in = append(in, c)
		var err error
		in, err = recurseLinks(bs, walked, c, in)
		if err != nil {
			rerr = err
		}
	})
	if err != nil {
		return nil, xerrors.Errorf("scanning for links failed: %w", err)
	}

	return in, rerr
}

func (store *Store) WalkSnapshot(ctx context.Context, ts *types.BlockHeader, inclRecentRoots abi.ChainEpoch, skipOldMsgs, skipMsgReceipts bool, cb func(cid.Cid) error) error {
	if ts == nil {
		ts = store.GetHead()
	}

	seen := cid.NewSet()
	walked := cid.NewSet()

	blocksToWalk := []cid.Cid{ts.Cid()}
	currentMinHeight := ts.Height

	walkChain := func(blk cid.Cid) error {
		if !seen.Visit(blk) {
			return nil
		}

		if err := cb(blk); err != nil {
			return err
		}

		data, err := store.bsstore.Get(blk)
		if err != nil {
			return xerrors.Errorf("getting block: %w", err)
		}

		var b types.BlockHeader
		if err := b.UnmarshalCBOR(bytes.NewBuffer(data.RawData())); err != nil {
			return xerrors.Errorf("unmarshaling block header (cid=%s): %w", blk, err)
		}

		if currentMinHeight > b.Height {
			currentMinHeight = b.Height
			if currentMinHeight%builtin.EpochsInDay == 0 {
				log.Infow("export", "height", currentMinHeight)
			}
		}

		var cids []cid.Cid
		if !skipOldMsgs || b.Height > ts.Height-inclRecentRoots {
			if walked.Visit(b.Messages) {
				mcids, err := recurseLinks(store.bsstore, walked, b.Messages, []cid.Cid{b.Messages})
				if err != nil {
					return xerrors.Errorf("recursing messages failed: %w", err)
				}
				cids = mcids
			}
		}

		if b.Height > 0 {
			blocksToWalk = append(blocksToWalk, b.Parent)
		} else {
			// include the genesis block
			cids = append(cids, b.Parent)
		}

		out := cids

		if b.Height == 0 || b.Height > ts.Height-inclRecentRoots {
			if walked.Visit(b.ParentStateRoot) {
				cids, err := recurseLinks(store.bsstore, walked, b.ParentStateRoot, []cid.Cid{b.ParentStateRoot})
				if err != nil {
					return xerrors.Errorf("recursing genesis state failed: %w", err)
				}

				out = append(out, cids...)
			}

			if !skipMsgReceipts && walked.Visit(b.ParentMessageReceipts) {
				out = append(out, b.ParentMessageReceipts)
			}
		}

		for _, c := range out {
			if seen.Visit(c) {
				if c.Prefix().Codec != cid.DagCBOR {
					continue
				}

				if err := cb(c); err != nil {
					return err
				}

			}
		}

		return nil
	}

	log.Infow("export started")
	exportStart := constants.Clock.Now()

	for len(blocksToWalk) > 0 {
		next := blocksToWalk[0]
		blocksToWalk = blocksToWalk[1:]
		if err := walkChain(next); err != nil {
			return xerrors.Errorf("walk chain failed: %w", err)
		}
	}

	log.Infow("export finished", "duration", constants.Clock.Now().Sub(exportStart).Seconds())

	return nil
}

//Import import a car file into local db
func (store *Store) Import(r io.Reader) (*types.BlockHeader, error) {
	header, err := car.LoadCar(store.bsstore, r)
	if err != nil {
		return nil, xerrors.Errorf("loadcar failed: %w", err)
	}
	headerRoot := header.Roots[0]
	root, err := store.GetBlock(nil, headerRoot)
	if err != nil {
		return nil, xerrors.Errorf("failed to load root tipset from chainfile: %w", err)
	}

	// Notice here is different with lotus, because the head tipset in lotus is not computed,
	// but in venus the head tipset is computed, so here we will fallback a pre tipset
	// and the chain store must has a metadata for each tipset, below code is to build the tipset metadata

	// Todo What to do if it is less than 900
	var (
		loopBack  = 900
		curTipset = root
	)

	log.Info("import height: ", root.Height, " root: ", root.String(), " parents: ", root.Parent)
	for i := 0; i < loopBack; i++ {
		if curTipset.Height <= 0 {
			break
		}
		curTipsetKey := curTipset.Parent
		curParentTipset, err := store.GetBlock(nil, curTipsetKey)
		if err != nil {
			return nil, xerrors.Errorf("failed to load root tipset from chainfile: %w", err)
		}

		if curParentTipset.Height == 0 {
			break
		}

		//save fake root
		/*err = store.PutTipSetMetadata(context.Background(), &TipSetMetadata{
			TipSetStateRoot: curTipset.At(0).ParentStateRoot,
			TipSet:          curParentTipset,
			TipSetReceipts:  curTipset.At(0).ParentMessageReceipts,
		})*/
		_, err = store.PutObject(nil, curTipset)
		if err != nil {
			return nil, err
		}
		curTipset = curParentTipset
	}

	if root.Height > 0 {
		root, err = store.GetBlock(nil, root.Parent)
		if err != nil {
			return nil, xerrors.Errorf("failed to load root tipset from chainfile: %w", err)
		}
	}
	return root, nil
}

func (store *Store) Export(ctx context.Context, ts *types.BlockHeader, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, w io.Writer) error {
	h := &car.CarHeader{
		Roots:   []cid.Cid{ts.Cid()},
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}

	return store.WalkSnapshot(ctx, ts, inclRecentRoots, skipOldMsgs, true, func(c cid.Cid) error {
		blk, err := store.bsstore.Get(c)
		if err != nil {
			return xerrors.Errorf("writing object to car, bs.Get: %w", err)
		}

		if err := carutil.LdWrite(w, c.Bytes(), blk.RawData()); err != nil {
			return xerrors.Errorf("failed to write block to car output: %w", err)
		}

		return nil
	})
}

// Store wrap adt store
func (store *Store) Store(ctx context.Context) adt.Store {
	return adt.WrapStore(ctx, cbor.NewCborStore(store.bsstore))
}

//StateCirculatingSupply get circulate supply at specify epoch
func (store *Store) StateCirculatingSupply(ctx context.Context, tsk cid.Cid) (abi.TokenAmount, error) {
	ts, err := store.GetBlock(ctx, tsk)
	if err != nil {
		return abi.TokenAmount{}, err
	}

	root, err := store.GetTipSetStateRoot(ts)
	if err != nil {
		return abi.TokenAmount{}, err
	}

	sTree, err := tree.LoadState(ctx, store.stateAndBlockSource, root)
	if err != nil {
		return abi.TokenAmount{}, err
	}

	return store.getCirculatingSupply(ctx, ts.Height, sTree)
}

func (store *Store) getCirculatingSupply(ctx context.Context, height abi.ChainEpoch, st tree.Tree) (abi.TokenAmount, error) {
	adtStore := adt.WrapStore(ctx, store.stateAndBlockSource)
	circ := big.Zero()
	unCirc := big.Zero()
	err := st.ForEach(func(a address.Address, actor *types.Actor) error {
		switch {
		case actor.Balance.IsZero():
			// Do nothing for zero-balance actors
			break
		case a == _init.Address ||
			a == reward.Address ||
			a == verifreg.Address ||
			// The power actor itself should never receive funds
			a == power.Address ||
			a == builtin.SystemActorAddr ||
			a == builtin.CronActorAddr ||
			a == builtin.BurntFundsActorAddr ||
			a == builtin.SaftAddress ||
			a == builtin.ReserveAddress:

			unCirc = big.Add(unCirc, actor.Balance)

		case a == market.Address:
			mst, err := market.Load(adtStore, actor)
			if err != nil {
				return err
			}

			lb, err := mst.TotalLocked()
			if err != nil {
				return err
			}

			circ = big.Add(circ, big.Sub(actor.Balance, lb))
			unCirc = big.Add(unCirc, lb)

		case builtin.IsAccountActor(actor.Code) || builtin.IsPaymentChannelActor(actor.Code):
			circ = big.Add(circ, actor.Balance)

		case builtin.IsStorageMinerActor(actor.Code):
			mst, err := miner.Load(adtStore, actor)
			if err != nil {
				return err
			}

			ab, err := mst.AvailableBalance(actor.Balance)

			if err == nil {
				circ = big.Add(circ, ab)
				unCirc = big.Add(unCirc, big.Sub(actor.Balance, ab))
			} else {
				// Assume any error is because the miner state is "broken" (lower actor balance than locked funds)
				// In this case, the actor's entire balance is considered "uncirculating"
				unCirc = big.Add(unCirc, actor.Balance)
			}

		case builtin.IsMultisigActor(actor.Code):
			mst, err := multisig.Load(adtStore, actor)
			if err != nil {
				return err
			}

			lb, err := mst.LockedBalance(height)
			if err != nil {
				return err
			}

			ab := big.Sub(actor.Balance, lb)
			circ = big.Add(circ, big.Max(ab, big.Zero()))
			unCirc = big.Add(unCirc, big.Min(actor.Balance, lb))
		default:
			return xerrors.Errorf("unexpected actor: %s", a)
		}

		return nil
	})

	if err != nil {
		return abi.TokenAmount{}, err
	}

	total := big.Add(circ, unCirc)
	if !total.Equals(types.TotalFilecoinInt) {
		return abi.TokenAmount{}, xerrors.Errorf("total filecoin didn't add to expected amount: %s != %s", total, types.TotalFilecoinInt)
	}

	return circ, nil
}

// WriteCheckPoint writes the given cids to disk.
func (store *Store) WriteCheckPoint(ctx context.Context, cids cid.Cid) error {
	log.Infof("WriteCheckPoint %v", cids)
	return store.ds.Put(CheckPoint, cids.Bytes())
}
