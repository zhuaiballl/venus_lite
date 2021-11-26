package syncer

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"sync"

	syncTypes "github.com/filecoin-project/venus_lite/pkg/chainsync/types"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/venus_lite/pkg/chain"
	"github.com/filecoin-project/venus_lite/pkg/chainsync/exchange"
	"github.com/filecoin-project/venus_lite/pkg/clock"
	"github.com/filecoin-project/venus_lite/pkg/constants"
	"github.com/filecoin-project/venus_lite/pkg/fork"
	"github.com/filecoin-project/venus_lite/pkg/metrics"
	"github.com/filecoin-project/venus_lite/pkg/metrics/tracing"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/filecoin-project/venus_lite/pkg/util/blockstoreutil"
)

// Syncer updates its chain.Store according to the methods of its
// consensus.Protocol.  It uses a bad tipset cache and a limit on new
// blocks to traverse during chain collection.  The Syncer can query the
// network for blocks.  The Syncer maintains the following invariant on
// its bsstore: all tipsets that pass the syncer's validity checks are added to the
// chain bsstore along with their state root CID.
//
// Ideally the code that syncs the chain according to consensus rules should
// be independent of any particular implementation of consensus.  Currently the
// Syncer is coupled to details of Expected Consensus. This dependence
// exists in the widen function, the fact that widen is called on only one
// tipset in the incoming chain, and assumptions regarding the existence of
// grandparent state in the bsstore.

var (
	// ErrForkTooLong is return when the syncing chain has fork with local
	ErrForkTooLong = fmt.Errorf("fork longer than threshold")
	// ErrChainHasBadTipSet is returned when the syncer traverses a chain with a cached bad tipset.
	ErrChainHasBadTipSet = errors.New("input chain contains a cached bad tipset")
	// ErrNewChainTooLong is returned when processing a fork that split off from the main chain too many blocks ago.
	ErrNewChainTooLong = errors.New("input chain forked from best chain past finality limit")
	// ErrUnexpectedStoreState indicates that the syncer's chain bsstore is violating expected invariants.
	ErrUnexpectedStoreState = errors.New("the chain bsstore is in an unexpected state")

	logSyncer    = logging.Logger("chainsync.syncer")
	syncOneTimer *metrics.Float64Timer
	reorgCnt     *metrics.Int64Counter // nolint
)

func init() {
	syncOneTimer = metrics.NewTimerMs("syncer/sync_one", "Duration of single tipset validation in milliseconds")
	reorgCnt = metrics.NewInt64Counter("chain/reorg_count", "The number of reorgs that have occurred.")
}

// StateProcessor does semantic validation on fullblocks.
type StateProcessor interface {
	// RunStateTransition returns the state root CID resulting from applying the input ts to the
	// prior `stateRoot`.  It returns an error if the transition is invalid.
	RunStateTransition(ctx context.Context, ts *types.BlockHeader, parentStateRoot cid.Cid) (root cid.Cid, receipt cid.Cid, err error)
}

//BlockValidator used to validate full block
type BlockValidator interface {
	ValidateFullBlock(ctx context.Context, blk *types.BlockHeader) error
}

// ChainReaderWriter reads and writes the chain bsstore.
type ChainReaderWriter interface {
	GetHead() *types.BlockHeader
	GetBlock(ctx context.Context, blockID cid.Cid) (*types.BlockHeader, error)
	//GetTipSet(types.TipSetKey) (*types.TipSet, error)
	//GetTipSetStateRoot(*types.TipSet) (cid.Cid, error)
	//GetTipSetReceiptsRoot(*types.TipSet) (cid.Cid, error)
	//HasTipSetAndState(context.Context, *types.TipSet) bool
	//PutTipSetMetadata(context.Context, *chain.TipSetMetadata) error
	SetHead(context.Context, *types.BlockHeader) error
	//HasSiblingState(*types.TipSet) bool
	//GetSiblingState(*types.TipSet) ([]*chain.TipSetMetadata, error)
	//GetLatestBeaconEntry(*types.TipSet) (*types.BeaconEntry, error)
	GetGenesisBlock(context.Context) (*types.BlockHeader, error)
	PutObject(context.Context, interface{}) (cid.Cid, error)
}

//messageStore used to save and load message from db
type messageStore interface {
	LoadTipSetMessage(ctx context.Context, ts *types.BlockHeader) ([]types.BlockMessagesInfo, error)
	LoadMetaMessages(context.Context, cid.Cid) ([]*types.SignedMessage, []*types.UnsignedMessage, error)
	LoadReceipts(context.Context, cid.Cid) ([]types.MessageReceipt, error)
	StoreReceipts(context.Context, []types.MessageReceipt) (cid.Cid, error)
}

// ChainSelector chooses the heaviest between chains.
type ChainSelector interface {
	// IsHeavier returns true if tipset a is heavier than tipset b and false if
	// tipset b is heavier than tipset a.
	IsHeavier(ctx context.Context, a, b *types.TipSet) (bool, error)
	// Weight returns the weight of a tipset after the upgrade to version 1
	Weight(ctx context.Context, ts *types.TipSet) (big.Int, error)
}

//Syncer used to synchronize the block from the specified target, including acquiring the relevant block data and message data,
//verifying the block machine messages one by one and calculating them, checking the weight of the target after the calculation,
//and check whether it can become the latest tipset
type Syncer struct {
	exchangeClient exchange.Client
	// BadTipSetCache is used to filter out collections of invalid blocks.
	badTipSets *syncTypes.BadTipSetCache

	// Evaluates tipset messages and stores the resulting states.
	stateProcessor StateProcessor
	// Validates headers and message structure
	blockValidator BlockValidator
	// Selects the heaviest of two chains
	chainSelector ChainSelector
	// Provides and stores validated tipsets and their state roots.
	chainStore ChainReaderWriter
	// Provides message collections given cids
	messageProvider messageStore

	clock    clock.Clock
	headLock sync.Mutex

	bsstore    blockstore.Blockstore
	checkPoint types.BlockHeader

	fork fork.IFork
}

// NewSyncer constructs a Syncer ready for use.  The chain reader must have a
// head tipset to initialize the staging field.
func NewSyncer(fv StateProcessor,
	hv BlockValidator,
	cs ChainSelector,
	s ChainReaderWriter,
	m messageStore,
	bsstore blockstore.Blockstore,
	exchangeClient exchange.Client,
	c clock.Clock,
	fork fork.IFork) (*Syncer, error) {

	if constants.InsecurePoStValidation {
		logSyncer.Warn("*********************************************************************************************")
		logSyncer.Warn(" [INSECURE-POST-VALIDATION] Insecure test validation is enabled. If you see this outside of a test, it is a severe bug! ")
		logSyncer.Warn("*********************************************************************************************")
	}
	return &Syncer{
		exchangeClient:  exchangeClient,
		badTipSets:      syncTypes.NewBadTipSetCache(),
		stateProcessor:  fv,
		blockValidator:  hv,
		chainSelector:   cs,
		bsstore:         bsstore,
		chainStore:      s,
		messageProvider: m,
		clock:           c,
		fork:            fork,
	}, nil
}

// syncOne syncs a single tipset with the chain bsstore. syncOne calculates the
// parent state of the tipset and calls into consensus to run a state transition
// in order to validate the tipset.  In the case the input tipset is valid,
// syncOne calls into consensus to check its weight, and then updates the head
// of the bsstore if this tipset is the heaviest.
//todo mark bad-block
func (syncer *Syncer) syncOne(ctx context.Context, parent *types.BlockHeader, next *types.BlockHeader) error {
	priorHead := syncer.chainStore.GetHead()
	// if tipset is already priorHeadKey, we've been here before. do nothing.
	if priorHead.Equals(next) {
		return nil
	}

	stopwatch := syncOneTimer.Start(ctx)
	defer stopwatch.Stop(ctx)

	// Lookup parent state and receipt root. It is guaranteed by the syncer that it is in the chainStore.
	//parentStateRoot, err := syncer.chainStore.GetTipSetStateRoot(parent)
	/*if err != nil {
		return xerrors.Errorf("get parent tipset state failed %w", err)
	}*/

	if !parent.Equals(&syncer.checkPoint) {
		var wg errgroup.Group

		wg.Go(func() error {
			// Fetch the URL.
			return syncer.blockValidator.ValidateFullBlock(ctx, next)
		})
		err := wg.Wait()
		if err != nil {
			return xerrors.Errorf("validate mining failed %w", err)
		}
	}
	// Run a state transition to validate the tipset and compute
	// a new state to add to the bsstore.
	/*toProcessTime := time.Now()
	root, receiptCid, err := syncer.stateProcessor.RunStateTransition(ctx, next, parentStateRoot)
	if err != nil {
		return xerrors.Errorf("calc current tipset %s state failed %w", next.Key().String(), err)
	}

	logSyncer.Infow("Process TipSet ", "Height:", next.Height(), "Blocks", next.Len(), " Root:", root, " receiptcid ", receiptCid, " time: ", time.Since(toProcessTime).Milliseconds())

	err = syncer.chainStore.PutTipSetMetadata(ctx, &chain.TipSetMetadata{
		TipSet:          next,
		TipSetStateRoot: root,
		TipSetReceipts:  receiptCid,
	})
	if err != nil {
		return err
	}*/
	syncer.chainStore.PutObject(ctx, next)
	return nil
}

// widen computes a tipset implied by the input tipset and the bsstore that
// could potentially be the heaviest tipset. In the context of EC, widen
// returns the union of the input tipset and the biggest tipset with the same
// parents from the bsstore.
// TODO: this leaks EC abstractions into the syncer, we should think about this.
// nolint
/*func (syncer *Syncer) widen(ctx context.Context, ts *types.TipSet) (*types.TipSet, error) {
	// Lookup tipsets with the same parents from the bsstore.
	if !syncer.chainStore.HasSiblingState(ts) {
		return nil, nil
	}
	//find current tipset base the same parent and height
	candidates, err := syncer.chainStore.GetSiblingState(ts)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Only take the tipset with the most blocks (this is EC specific logic) todo ?
	max := candidates[0].TipSet
	for _, candidate := range candidates[0:] {
		if candidate.TipSet.Len() > max.Len() {
			max = candidate.TipSet
		}
	}

	// Form a new tipset from the union of ts and the largest in the bsstore, de-duped.
	var blockSlice []*types.BlockHeader
	blockCids := make(map[cid.Cid]struct{})
	for i := 0; i < ts.Len(); i++ {
		blk := ts.At(i)
		blockCids[blk.Cid()] = struct{}{}
		blockSlice = append(blockSlice, blk)
	}
	for i := 0; i < max.Len(); i++ {
		blk := max.At(i)
		if _, found := blockCids[blk.Cid()]; !found {
			blockSlice = append(blockSlice, blk)
			blockCids[blk.Cid()] = struct{}{}
		}
	}
	wts, err := types.NewTipSet(blockSlice...)
	if err != nil {
		return nil, err
	}

	// check that the tipset is distinct from the input and tipsets from the bsstore.
	if wts.String() == ts.String() || wts.String() == max.String() {
		return nil, nil
	}

	return wts, nil
}*/

// HandleNewTipSet validates and syncs the chain rooted at the provided tipset
// to a chain bsstore.  Iff catchup is false then the syncer will set the head.
func (syncer *Syncer) HandleNewTipSet(ctx context.Context, target *syncTypes.Target) (err error) {
	ctx, span := trace.StartSpan(ctx, "Syncer.HandleNewTipSet")
	span.AddAttributes(trace.StringAttribute("block", target.Head.String()))
	defer func() {
		if err != nil {
			target.Err = err
			target.State = syncTypes.StageSyncErrored
		} else {
			target.State = syncTypes.StageSyncComplete
		}
		tracing.AddErrorEndSpan(ctx, span, &err)
	}()
	logSyncer.Infof("Begin fetch and sync of chain with head %v from %s at height %v", target.Head.Cid().String(), target.Sender.String(), target.Head.Height)
	head := syncer.chainStore.GetHead()
	//If the store already has this tipset then the syncer is finished.
	if target.Head.ParentWeight.LessThan(head.ParentWeight) {
		return xerrors.New("do not sync to a target with less weight")
	}

	if target.Head.Cid().Equals(head.Cid()) {
		return xerrors.New("do not sync to a target has synced before")
	}

	blockheaders, err := syncer.fetchChainBlocks(ctx, head, target.Head)
	//e.g.blockheader[]={height:4,height:5,height:6}
	if err != nil {
		return errors.Wrapf(err, "failure fetching or validating headers")
	}

	logSyncer.Infof("fetch header success at %v %s ...", blockheaders[0].Height, blockheaders[0].Cid().String())
	return syncer.syncSegement(ctx, target, blockheaders)
}

func (syncer *Syncer) syncSegement(ctx context.Context, target *syncTypes.Target, headers []*types.BlockHeader) error {
	parent, err := syncer.chainStore.GetBlock(ctx, headers[0].Parent)
	if err != nil {
		return err
	}

	errProcessChan := make(chan error, 1)
	errProcessChan <- nil //init
	var wg sync.WaitGroup
	//todo  write a pipline segment processor function
	if err = rangeProcess(headers, func(segHeaders []*types.BlockHeader) error {
		// fetch messages
		startTip := segHeaders[0].Height
		emdTipset := segHeaders[len(segHeaders)-1].Height
		logSyncer.Infof("start to fetch message segement %d-%d", startTip, emdTipset)
		_, err := syncer.fetchSegMessage(ctx, segHeaders)
		if err != nil {
			return err
		}
		logSyncer.Infof("finish to fetch message segement %d-%d", startTip, emdTipset)
		err = <-errProcessChan
		if err != nil {
			return xerrors.Errorf("process message failed %v", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			logSyncer.Infof("start to process message segement %d-%d", startTip, emdTipset)
			defer logSyncer.Infof("finish to process message segement %d-%d", startTip, emdTipset)
			var processErr error
			parent, processErr = syncer.processTipSetSegment(ctx, target, parent, segHeaders)
			if processErr != nil {
				errProcessChan <- processErr
				return
			}

			if !parent.Equals(&syncer.checkPoint) {
				err := syncer.SetHead(ctx, parent)
				if err != nil {
					errProcessChan <- err
					return
				}
			}
			errProcessChan <- nil
		}()
		return nil
	}); err != nil {
		return err
	}

	wg.Wait()
	select {
	case err = <-errProcessChan:
		return err
	default:
		return nil
	}
}

//fetchChainBlocks get the block data, from targettip to knowntip.
// if local db has the block used that block
// if local db not exist, get block from network(libp2p),
// if there is a fork, get the common root tipset of knowntip and targettip, and return the block data from root tipset to targettip
// local(···->A->B) + incoming(C->D->E)  => ···->A->B->C->D->E
func (syncer *Syncer) fetchChainBlocks(ctx context.Context, knownTip *types.BlockHeader, targetTip *types.BlockHeader) ([]*types.BlockHeader, error) {
	chainBlocks := []*types.BlockHeader{targetTip}
	var flushDB = func(saveTips []*types.BlockHeader) error {
		bs := blockstoreutil.NewTemporary()
		cborStore := cbor.NewCborStore(bs)
		for _, blk := range saveTips {
			_, err := cborStore.Put(ctx, blk)
			if err != nil {
				return err
			}
		}
		return blockstoreutil.CopyBlockstore(ctx, bs, syncer.bsstore)
	}

	untilHeight := knownTip.Height
	count := 0
loop:
	for chainBlocks[len(chainBlocks)-1].Height > untilHeight {
		tipSet, err := syncer.chainStore.GetBlock(ctx, targetTip.Parent)
		if err == nil {
			chainBlocks = append(chainBlocks, tipSet)
			targetTip = tipSet
			count++
			if count%500 == 0 {
				logSyncer.Info("load from local db ", "Height: ", tipSet.Height)
			}
			continue
		}

		windows := targetTip.Height - untilHeight
		if windows > 500 {
			windows = 500
		}

		fetchHeaders, err := syncer.exchangeClient.GetBlocks(ctx, targetTip.Parent, int(windows))
		if err != nil {
			return nil, err
		}

		if len(fetchHeaders) == 0 {
			break loop
		}

		logSyncer.Infof("fetch blocks %d height from %d-%d", len(fetchHeaders), fetchHeaders[0].Height, fetchHeaders[len(fetchHeaders)-1].Height)
		if err = flushDB(fetchHeaders); err != nil {
			return nil, err
		}
		for _, b := range fetchHeaders {
			if b.Height < untilHeight {
				break loop
			}
			chainBlocks = append(chainBlocks, b)
			targetTip = b
		}
	}

	base := chainBlocks[len(chainBlocks)-1]
	if base.Equals(knownTip) {
		chainBlocks = chainBlocks[:len(chainBlocks)-1]
		base = chainBlocks[len(chainBlocks)-1]
	}

	/*if base.IsChildOf(knownTip) {
		// common case: receiving blocks that are building on top of our best tipset
		chain.Reverse(chainTipsets)
		return chainTipsets, nil
	}*/

	/*knownParent, err := syncer.chainStore.GetTipSet(knownTip.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to load next local tipset: %w", err)
	}
	if base.IsChildOf(knownParent) {
		// common case: receiving a block thats potentially part of the same tipset as our best block
		chain.Reverse(chainTipsets)
		return chainTipsets, nil
	}

	logSyncer.Warnf("(fork detected) synced header chain")
	fork, err := syncer.syncFork(ctx, base, knownTip)
	if err != nil {
		if xerrors.Is(err, ErrForkTooLong) {
			// TODO: we're marking this block bad in the same way that we mark invalid blocks bad. Maybe distinguish?
			logSyncer.Warn("adding forked chain to our bad tipset cache")
		}
		return nil, xerrors.Errorf("failed to sync fork: %w", err)
	}
	err = flushDB(fork)
	if err != nil {
		return nil, err
	}
	chainTipsets = append(chainTipsets, fork...)*/
	chain.Reverse(chainBlocks)
	return chainBlocks, nil
}

// syncFork tries to obtain the chain fragment that links a fork into a common
// ancestor in our view of the chain.
//
// If the fork is too long (build.ForkLengthThreshold), or would cause us to diverge from the checkpoint (ErrForkCheckpoint),
// we add the entire subchain to the denylist. Else, we find the common ancestor, and add the missing chain
// fragment until the fork point to the returned []TipSet.
//
//		D->E-F(targetTip）
//	A						 => D->E>F
//		B-C(knownTip)
//
/*func (syncer *Syncer) syncFork(ctx context.Context, incoming *types.TipSet, known *types.TipSet) ([]*types.TipSet, error) {
	// TODO: Does this mean we always ask for ForkLengthThreshold blocks from the network, even if we just need, like, 2?
	// Would it not be better to ask in smaller chunks, given that an ~ForkLengthThreshold is very rare?
	tips, err := syncer.exchangeClient.GetBlocks(ctx, incoming.Parents(), int(policy.ChainFinality))
	if err != nil {
		return nil, err
	}

	gensisiBlock, err := syncer.chainStore.GetGenesisBlock(ctx)
	if err != nil {
		return nil, err
	}

	nts, err := syncer.chainStore.GetTipSet(known.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to load next local tipset: %w", err)
	}

	for cur := 0; cur < len(tips); {
		if nts.Height() == 0 {
			if !gensisiBlock.Equals(nts.At(0)) {
				return nil, xerrors.Errorf("somehow synced chain that linked back to a different genesis (bad genesis: %s)", nts.Key())
			}
			return nil, xerrors.Errorf("synced chain forked at genesis, refusing to sync; incoming: %s", incoming.ToSlice())
		}

		if nts.Equals(tips[cur]) {
			return tips[:cur], nil
		}

		if nts.Height() < tips[cur].Height() {
			cur++
		} else {
			nts, err = syncer.chainStore.GetTipSet(nts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("loading next local tipset: %w", err)
			}
		}
	}

	return nil, ErrForkTooLong
}*/

//fetchSegMessage get message in tipset
func (syncer *Syncer) fetchSegMessage(ctx context.Context, segHeaders []*types.BlockHeader) ([]*types.FullBlock, error) {
	//get message from local bsstore
	if len(segHeaders) == 0 {
		return []*types.FullBlock{}, nil
	}

	chain.Reverse(segHeaders)
	//e.g.blockheader[]={height:6,height:5,height:4}
	defer chain.Reverse(segHeaders)
	//defer: e.g.blockheader[]={height:4,height:5,height:6}

	fullBlocks := make([]*types.FullBlock, len(segHeaders))
	defer types.ReverseFullBlock(fullBlocks)

	var leftChain []*types.BlockHeader
	//var leftFullChain []*types.FullBlock
	for index, tip := range segHeaders {
		fullBlock, err := syncer.getFullBlock(ctx, tip)
		if err != nil {
			leftChain = segHeaders[index:]
			//leftFullChain = fullBlocks[index:]
			break
		}
		fullBlocks[index] = fullBlock
	}

	if len(leftChain) == 0 {
		return fullBlocks, nil
	}
	// fetch message from remote nodes
	bs := blockstoreutil.NewTemporary()
	cborStore := cbor.NewCborStore(bs)

	messages, err := syncer.exchangeClient.GetChainMessages(ctx, leftChain)
	if err != nil {
		return nil, err
	}
	types.ReverseFullBlock(messages)
	for _, tip := range messages {
		/*fts, err := zipTipSetAndMessages(bs, tip, messages[index].Bls, messages[index].Secpk, messages[index].BlsIncludes, messages[index].SecpkIncludes)
		if err != nil {
			return nil, xerrors.Errorf("message processing failed: %w", err)
		}
		leftFullChain[index] = fts*/

		//save message
		if _, err := cborStore.Put(ctx, tip.BLSMessages); err != nil {
			return nil, xerrors.Errorf("BLS message processing failed: %w", err)
		}
		if _, err := cborStore.Put(ctx, tip.SECPMessages); err != nil {
			return nil, xerrors.Errorf("SECP message processing failed: %w", err)
		}
		fullBlocks = append(fullBlocks, tip)
	}

	err = blockstoreutil.CopyBlockstore(ctx, bs, syncer.bsstore)
	if err != nil {
		return nil, errors.Wrapf(err, "failure fetching full blocks")
	}
	return fullBlocks, nil
}

//getFullBlock get full block from message store
func (syncer *Syncer) getFullBlock(ctx context.Context, bh *types.BlockHeader) (*types.FullBlock, error) {
	//fullBlocks := make([]*types.FullBlock, tipset.Len())
	secpMsg, blsMsg, err := syncer.messageProvider.LoadMetaMessages(ctx, bh.Messages)
	if err != nil {
		return nil, err
	}
	FB := types.FullBlock{
		Header:       bh,
		BLSMessages:  blsMsg,
		SECPMessages: secpMsg,
	}

	return &FB, nil
}

//processTipSetSegment process a batch of tipset in turn，
func (syncer *Syncer) processTipSetSegment(ctx context.Context, target *syncTypes.Target, parent *types.BlockHeader, segHeaders []*types.BlockHeader) (*types.BlockHeader, error) {
	for i, bh := range segHeaders {
		err := syncer.syncOne(ctx, parent, bh)
		if err != nil {
			// While `syncOne` can indeed fail for reasons other than consensus,
			// adding to the badTipSets at this point is the simplest, since we
			// have access to the chain. If syncOne fails for non-consensus reasons,
			// there is no assumption that the running node's data is valid at all,
			// so we don't really lose anything with this simplification.
			//syncer.badTipSets.AddChain(segTipset[i:])
			return nil, errors.Wrapf(err, "failed to sync blocks %s, number %d of %d in chain", bh.Cid().String(), i, len(segHeaders))
		}
		parent = bh
		target.Current = bh
	}
	return parent, nil
}

//Head get latest head from chain store
func (syncer *Syncer) Head() *types.BlockHeader {
	return syncer.chainStore.GetHead()
}

//SetHead try to sethead after complete tipset syncing,
// if the current target weight is heavier than chain store. change a new head
func (syncer *Syncer) SetHead(ctx context.Context, newHead *types.BlockHeader) error {
	syncer.headLock.Lock()
	defer syncer.headLock.Unlock()
	head := syncer.chainStore.GetHead()
	/*heavier, err := syncer.chainSelector.IsHeavier(ctx, ts, head)
	if err != nil {
		return err
	}*/

	// If it is the heaviest update the chainStore.
	if newHead.Height > head.Height {
		/*exceeds, err := syncer.exceedsForkLength(head, ts)
		if err != nil {
			return err
		}
		if exceeds {
			return nil
		}*/
		return syncer.chainStore.SetHead(ctx, newHead)
	}
	return nil
}

// Check if the two tipsets have a fork length above `ForkLengthThreshold`.
// `synced` is the head of the chain we are currently synced to and `external`
// is the incoming tipset potentially belonging to a forked chain. It assumes
// the external chain has already been validated and available in the ChainStore.
// The "fast forward" case is covered in this logic as a valid fork of length 0.
//
// FIXME: We may want to replace some of the logic in `syncFork()` with this.
//  `syncFork()` counts the length on both sides of the fork at the moment (we
//  need to settle on that) but here we just enforce it on the `synced` side.
/*func (syncer *Syncer) exceedsForkLength(synced, external *types.TipSet) (bool, error) {
	if synced == nil || external == nil {
		// FIXME: If `cs.heaviest` is nil we should just bypass the entire
		//  `MaybeTakeHeavierTipSet` logic (instead of each of the called
		//  functions having to handle the nil case on their own).
		return false, nil
	}

	var err error
	// `forkLength`: number of tipsets we need to walk back from the our `synced`
	// chain to the common ancestor with the new `external` head in order to
	// adopt the fork.
	for forkLength := 0; forkLength < int(constants.ForkLengthThreshold); forkLength++ {
		// First walk back as many tipsets in the external chain to match the
		// `synced` height to compare them. If we go past the `synced` height
		// the subsequent match will fail but it will still be useful to get
		// closer to the `synced` head parent's height in the next loop.
		for external.Height() > synced.Height() {
			if external.Height() == 0 {
				// We reached the genesis of the external chain without a match;
				// this is considered a fork outside the allowed limit (of "infinite"
				// length).
				return true, nil
			}

			external, err = syncer.chainStore.GetTipSet(external.Parents())
			if err != nil {
				return false, xerrors.Errorf("failed to load parent tipset in external chain: %w", err)
			}
		}

		// Now check if we arrived at the common ancestor.
		if synced.Equals(external) {
			return false, nil
		}

		// Now check to see if we've walked back to the checkpoint.
		if synced.Key().Equals(syncer.checkPoint) {
			return true, nil
		}

		// If we didn't, go back *one* tipset on the `synced` side (incrementing
		// the `forkLength`).
		if synced.Height() == 0 {
			// Same check as the `external` side, if we reach the start (genesis)
			// there is no common ancestor.
			return true, nil
		}
		synced, err = syncer.chainStore.GetTipSet(synced.Parents())
		if err != nil {
			return false, xerrors.Errorf("failed to load parent tipset in synced chain: %w", err)
		}
	}

	// We traversed the fork length allowed without finding a common ancestor.
	return true, nil
}*/

// TODO: this function effectively accepts unchecked input from the network,
// either validate it here, or ensure that its validated elsewhere (maybe make
// sure the blocksync code checks it?)
// maybe this code should actually live in blocksync??
/*func zipTipSetAndMessages(bs blockstore.Blockstore, ts *types.TipSet, allbmsgs []*types.UnsignedMessage, allsmsgs []*types.SignedMessage, bmi, smi [][]uint64) (*types.FullTipSet, error) {
	if len(ts.Blocks()) != len(smi) || len(ts.Blocks()) != len(bmi) {
		return nil, fmt.Errorf("msgincl length didnt match tipset size")
	}

	fts := &types.FullTipSet{}
	for bi, b := range ts.Blocks() {
		if msgc := len(bmi[bi]) + len(smi[bi]); msgc > constants.BlockMessageLimit {
			return nil, fmt.Errorf("block %q has too many messages (%d)", b.Cid(), msgc)
		}

		var smsgs []*types.SignedMessage
		var smsgCids []cid.Cid
		for _, m := range smi[bi] {
			smsgs = append(smsgs, allsmsgs[m])
			mCid := allsmsgs[m].Cid()
			smsgCids = append(smsgCids, mCid)
		}

		var bmsgs []*types.UnsignedMessage
		var bmsgCids []cid.Cid
		for _, m := range bmi[bi] {
			bmsgs = append(bmsgs, allbmsgs[m])
			mCid := allbmsgs[m].Cid()
			bmsgCids = append(bmsgCids, mCid)
		}

		mrcid, err := chain.ComputeMsgMeta(bs, bmsgCids, smsgCids)
		if err != nil {
			return nil, err
		}

		if b.Messages != mrcid {
			return nil, fmt.Errorf("messages didnt match message root in header for ts %s", ts.Key())
		}

		fb := &types.FullBlock{
			Header:       b,
			BLSMessages:  bmsgs,
			SECPMessages: smsgs,
		}

		fts.Blocks = append(fts.Blocks, fb)
	}

	return fts, nil
}*/

const maxProcessLen = 32

func rangeProcess(bh []*types.BlockHeader, cb func(bh []*types.BlockHeader) error) (err error) {
	for {
		if len(bh) == 0 {
			break
		} else if len(bh) < maxProcessLen {
			// break out if less than process len
			err = cb(bh)
			break
		} else {
			processTS := bh[0:maxProcessLen]
			err = cb(processTS)
			if err != nil {
				break
			}
			bh = bh[maxProcessLen:]
		}
		logSyncer.Infof("Sync Process End,Remaining: %v, err: %v ...", len(bh), err)
	}
	return err
}
