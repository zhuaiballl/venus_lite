package apitypes

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin"
	"github.com/ipfs/go-cid"
)

type MiningBaseInfo struct { //nolint
	MinerPower        abi.StoragePower
	NetworkPower      abi.StoragePower
	Sectors           []builtin.SectorInfo
	WorkerKey         address.Address
	SectorSize        abi.SectorSize
	PrevBeaconEntry   types.BeaconEntry
	BeaconEntries     []types.BeaconEntry
	EligibleForMining bool
}

type BlockTemplate struct {
	Miner  address.Address
	Parent cid.Cid
	Ticket types.Ticket
	//Eproof           *types.ElectionProof
	BeaconValues     []*types.BeaconEntry
	Messages         []*types.SignedMessage
	Epoch            abi.ChainEpoch
	Timestamp        uint64
	WinningPoStProof []proof2.PoStProof
}
