package chain

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus_lite/pkg/state"
	"github.com/filecoin-project/venus_lite/pkg/state/tree"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/policy"
	"github.com/filecoin-project/venus_lite/pkg/util"
	"golang.org/x/xerrors"
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
