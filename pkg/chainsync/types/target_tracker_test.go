package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	fbig "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"
	"gotest.tools/assert"
	"math/rand"
	"testing"
	"time"
)

func TestTargetTracker_Merge(t *testing.T) {

	targetTracker := NewTargetTracker(10)

	baseTipset, _ := types.NewTipSet(newTestBlock("base", 0, 0))
	targetTracker.Add(&Target{
		State:   StateInSyncing,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   baseTipset,
		},
	})

	t1, _ := types.NewTipSet(newTestBlock("base1", 1, 1))
	t2, _ := types.NewTipSet(newTestBlock("base2", 1, 1))

	toSync := targetTracker.Add(&Target{
		State:   StageIdle,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   t1,
		},
	})
	if !toSync {
		t.Errorf("expect to sync a new target")
	}

	toSync = targetTracker.Add(&Target{
		State:   StageIdle,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   t2,
		},
	})
	if !toSync {
		t.Errorf("expect to sync a merged target")
	}

	if targetTracker.Len() != 2 {
		t.Errorf("expect %d target but got %d", 2, targetTracker.Len())
	}
}

func TestTargetTracker_Abandon(t *testing.T) {

	targetTracker := NewTargetTracker(10)

	baseTipset, _ := types.NewTipSet(newTestBlock("base", 0, 1))
	targetTracker.Add(&Target{
		State:   StateInSyncing,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   baseTipset,
		},
	})

	t1, _ := types.NewTipSet(newTestBlock("base1", 1, 0))

	toSync := targetTracker.Add(&Target{
		State:   StateInSyncing,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   t1,
		},
	})
	if toSync {
		t.Errorf("expect not sync a low weight target")
	}

	if targetTracker.Len() != 1 {
		t.Errorf("expect %d target but got %d", 2, targetTracker.Len())
	}
}

func TestTargetTracker_ReplaceIdle(t *testing.T) {

	targetTracker := NewTargetTracker(10)

	baseTipset, _ := types.NewTipSet(newTestBlock("base", 0, 1))
	targetTracker.Add(&Target{
		State:   StageIdle,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   baseTipset,
		},
	})

	t1, _ := types.NewTipSet(newTestBlock("base1", 1, 1))

	toSync := targetTracker.Add(&Target{
		State:   StageIdle,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   t1,
		},
	})
	if toSync {
		t.Errorf("expect not sync a low weight target")
	}

	if targetTracker.Len() != 1 {
		t.Errorf("expect %d target but got %d", 2, targetTracker.Len())
	}
	target, has := targetTracker.Select()

	assert.Equal(t, has, true)
	assert.Equal(t, target.Head.Len(), 1)
	assert.Equal(t, target.Head.Height(), abi.ChainEpoch(2))

}

func TestTargetTracker_Append(t *testing.T) {

	targetTracker := NewTargetTracker(10)

	baseTipset, _ := types.NewTipSet(newTestBlock("base", 0, 0))
	targetTracker.Add(&Target{
		State:   StateInSyncing,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   baseTipset,
		},
	})

	t1, _ := types.NewTipSet(newTestBlock("base1", 1, 1))
	t2, _ := types.NewTipSet(newTestBlock("base2", 2, 2))

	toSync := targetTracker.Add(&Target{
		State:   StateInSyncing,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   t1,
		},
	})
	if !toSync {
		t.Errorf("expect to sync a new target")
	}

	toSync = targetTracker.Add(&Target{
		State:   StateInSyncing,
		Base:    baseTipset,
		Current: nil,
		Start:   time.Time{},
		End:     time.Time{},
		Err:     nil,
		ChainInfo: types.ChainInfo{
			Source: "",
			Sender: "",
			Head:   t2,
		},
	})
	if !toSync {
		t.Errorf("expect to sync a merged target")
	}

	if targetTracker.Len() != 3 {
		t.Errorf("expect %d target but got %d", 3, targetTracker.Len())
	}
}

func newTestBlock(randStr string, height abi.ChainEpoch, weight int64) *types.BlockHeader {
	rand.Seed(time.Now().Unix())
	mAddr, _ := address.NewActorAddress([]byte(randStr))
	cid, _ := cid.Decode("bafy2bzacechdx6xd62lcyy7rnyc4uxcxhuwqslcxfvj77fxlwafij3nhzchpy")
	return &types.BlockHeader{
		Miner:                 mAddr,
		Ticket:                types.Ticket{},
		ElectionProof:         nil,
		BeaconEntries:         nil,
		WinPoStProof:          nil,
		Parents:               types.TipSetKey{},
		ParentWeight:          fbig.NewInt(weight),
		Height:                height,
		ParentStateRoot:       cid,
		ParentMessageReceipts: cid,
		Messages:              cid,
		BLSAggregate:          nil,
		Timestamp:             0,
		BlockSig:              nil,
		ForkSignaling:         0,
		ParentBaseFee:         abi.TokenAmount{},
	}
}
