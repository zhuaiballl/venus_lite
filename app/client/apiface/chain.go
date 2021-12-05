package apiface

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	"github.com/filecoin-project/venus_lite/pkg/chain"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/miner"

	"github.com/filecoin-project/go-state-types/big"
	acrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/ipfs/go-cid"
)

type IChain interface {
	IAccount
	IActor
	IBeacon
	IMinerState
	IChainInfo
}

type IAccount interface {
	// Rule[perm:read]
	StateAccountKey(ctx context.Context, addr address.Address, tsk cid.Cid) (address.Address, error)
}

type IActor interface {
	// Rule[perm:read]
	StateGetActor(ctx context.Context, actor address.Address, tsk cid.Cid) (*types.Actor, error)
	// Rule[perm:read]
	ListActor(ctx context.Context) (map[address.Address]*types.Actor, error)
}

type IBeacon interface {
	// Rule[perm:read]
	BeaconGetEntry(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error)
}

type IChainInfo interface {
	// Rule[perm:read]
	BlockTime(ctx context.Context) time.Duration
	// Rule[perm:read]
	ChainList(ctx context.Context, tsKey cid.Cid, count int) ([]cid.Cid, error)
	// Rule[perm:read]
	ChainHead(ctx context.Context) (*types.BlockHeader, error)
	// Rule[perm:read]
	ChainSetHead(ctx context.Context, key cid.Cid) error
	// Rule[perm:read]
	ChainGetTipSet(ctx context.Context, key cid.Cid) (*types.BlockHeader, error)
	// Rule[perm:read]
	ChainGetTipSetByHeight(ctx context.Context, height abi.ChainEpoch, tsk cid.Cid) (*types.BlockHeader, error)
	// Rule[perm:read]
	ChainGetTipSetAfterHeight(ctx context.Context, height abi.ChainEpoch, tsk cid.Cid) (*types.BlockHeader, error)
	// Rule[perm:read]
	ChainGetRandomnessFromBeacon(ctx context.Context, key cid.Cid, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	// Rule[perm:read]
	ChainGetRandomnessFromTickets(ctx context.Context, tsk cid.Cid, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	// Rule[perm:read]
	StateGetRandomnessFromTickets(ctx context.Context, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk cid.Cid) (abi.Randomness, error)
	// Rule[perm:read]
	StateGetRandomnessFromBeacon(ctx context.Context, personalization acrypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk cid.Cid) (abi.Randomness, error)
	// Rule[perm:read]
	ChainGetBlock(ctx context.Context, id cid.Cid) (*types.BlockHeader, error)
	// Rule[perm:read]
	ChainGetMessage(ctx context.Context, msgID cid.Cid) (*types.UnsignedMessage, error)
	// Rule[perm:read]
	ChainGetBlockMessages(ctx context.Context, bid cid.Cid) (*apitypes.BlockMessages, error)
	// Rule[perm:read]
	ChainGetMessagesInTipset(ctx context.Context, key cid.Cid) ([]apitypes.Message, error)
	// Rule[perm:read]
	ChainGetReceipts(ctx context.Context, id cid.Cid) ([]types.MessageReceipt, error)
	// Rule[perm:read]
	ChainGetParentMessages(ctx context.Context, bcid cid.Cid) ([]apitypes.Message, error)
	// Rule[perm:read]
	ChainGetParentReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error)
	// Rule[perm:read]
	StateVerifiedRegistryRootKey(ctx context.Context, tsk cid.Cid) (address.Address, error)
	// Rule[perm:read]
	StateVerifierStatus(ctx context.Context, addr address.Address, tsk cid.Cid) (*abi.StoragePower, error)
	// Rule[perm:read]
	ChainNotify(ctx context.Context) <-chan []*chain.HeadChange
	// Rule[perm:read]
	GetFullBlock(ctx context.Context, id cid.Cid) (*types.FullBlock, error)
	// Rule[perm:read]
	GetActor(ctx context.Context, addr address.Address) (*types.Actor, error)
	// Rule[perm:read]
	GetParentStateRootActor(ctx context.Context, ts *types.BlockHeader, addr address.Address) (*types.Actor, error)
	// Rule[perm:read]
	GetEntry(ctx context.Context, height abi.ChainEpoch, round uint64) (*types.BeaconEntry, error)
	// Rule[perm:read]
	MessageWait(ctx context.Context, msgCid cid.Cid, confidence, lookback abi.ChainEpoch) (*chain.ChainMessage, error)
	// Rule[perm:read]
	ProtocolParameters(ctx context.Context) (*apitypes.ProtocolParams, error)
	// Rule[perm:read]
	ResolveToKeyAddr(ctx context.Context, addr address.Address, ts *types.BlockHeader) (address.Address, error)
	// Rule[perm:read]
	StateNetworkName(ctx context.Context) (apitypes.NetworkName, error)
	// StateSearchMsg looks back up to limit epochs in the chain for a message, and returns its receipt and the tipset where it was executed
	//
	// NOTE: If a replacing message is found on chain, this method will return
	// a MsgLookup for the replacing message - the MsgLookup.Message will be a different
	// CID than the one provided in the 'cid' param, MsgLookup.Receipt will contain the
	// result of the execution of the replacing message.
	//
	// If the caller wants to ensure that exactly the requested message was executed,
	// they must check that MsgLookup.Message is equal to the provided 'cid', or set the
	// `allowReplaced` parameter to false. Without this check, and with `allowReplaced`
	// set to true, both the requested and original message may appear as
	// successfully executed on-chain, which may look like a double-spend.
	//
	// A replacing message is a message with a different CID, any of Gas values, and
	// different signature, but with all other parameters matching (source/destination,
	// nonce, params, etc.)
	// Rule[perm:read]
	StateSearchMsg(ctx context.Context, from cid.Cid, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*apitypes.MsgLookup, error)
	// StateWaitMsg looks back up to limit epochs in the chain for a message.
	// If not found, it blocks until the message arrives on chain, and gets to the
	// indicated confidence depth.
	//
	// NOTE: If a replacing message is found on chain, this method will return
	// a MsgLookup for the replacing message - the MsgLookup.Message will be a different
	// CID than the one provided in the 'cid' param, MsgLookup.Receipt will contain the
	// result of the execution of the replacing message.
	//
	// If the caller wants to ensure that exactly the requested message was executed,
	// they must check that MsgLookup.Message is equal to the provided 'cid', or set the
	// `allowReplaced` parameter to false. Without this check, and with `allowReplaced`
	// set to true, both the requested and original message may appear as
	// successfully executed on-chain, which may look like a double-spend.
	//
	// A replacing message is a message with a different CID, any of Gas values, and
	// different signature, but with all other parameters matching (source/destination,
	// nonce, params, etc.)
	// Rule[perm:read]
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*apitypes.MsgLookup, error)
	// Rule[perm:read]
	StateNetworkVersion(ctx context.Context, tsk cid.Cid) (network.Version, error)
	// Rule[perm:read]
	VerifyEntry(parent, child *types.BeaconEntry, height abi.ChainEpoch) bool
	// Rule[perm:read]
	ChainExport(context.Context, abi.ChainEpoch, bool, cid.Cid) (<-chan []byte, error)
	// Rule[perm:read]
	ChainGetPath(ctx context.Context, from cid.Cid, to cid.Cid) ([]*chain.HeadChange, error)
}

type IMinerState interface {
	// Rule[perm:read]
	StateMinerSectorAllocated(ctx context.Context, maddr address.Address, s abi.SectorNumber, tsk cid.Cid) (bool, error)
	// Rule[perm:read]
	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk cid.Cid) (miner.SectorPreCommitOnChainInfo, error)
	// Rule[perm:read]
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk cid.Cid) (*miner.SectorOnChainInfo, error)
	// Rule[perm:read]
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk cid.Cid) (*miner.SectorLocation, error)
	// Rule[perm:read]
	StateMinerSectorSize(ctx context.Context, maddr address.Address, tsk cid.Cid) (abi.SectorSize, error)
	// Rule[perm:read]
	StateMinerInfo(ctx context.Context, maddr address.Address, tsk cid.Cid) (miner.MinerInfo, error)
	// Rule[perm:read]
	StateMinerWorkerAddress(ctx context.Context, maddr address.Address, tsk cid.Cid) (address.Address, error)
	// Rule[perm:read]
	StateMinerRecoveries(ctx context.Context, maddr address.Address, tsk cid.Cid) (bitfield.BitField, error)
	// Rule[perm:read]
	StateMinerFaults(ctx context.Context, maddr address.Address, tsk cid.Cid) (bitfield.BitField, error)
	// Rule[perm:read]
	StateMinerProvingDeadline(ctx context.Context, maddr address.Address, tsk cid.Cid) (*dline.Info, error)
	// Rule[perm:read]
	StateMinerPartitions(ctx context.Context, maddr address.Address, dlIdx uint64, tsk cid.Cid) ([]apitypes.Partition, error)
	// Rule[perm:read]
	StateMinerDeadlines(ctx context.Context, maddr address.Address, tsk cid.Cid) ([]apitypes.Deadline, error)
	// Rule[perm:read]
	StateMinerSectors(ctx context.Context, maddr address.Address, sectorNos *bitfield.BitField, tsk cid.Cid) ([]*miner.SectorOnChainInfo, error)
	// Rule[perm:read]
	StateMarketStorageDeal(ctx context.Context, dealID abi.DealID, tsk cid.Cid) (*apitypes.MarketDeal, error)
	// Rule[perm:read]
	StateMinerPreCommitDepositForPower(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk cid.Cid) (big.Int, error)
	// Rule[perm:read]
	StateMinerInitialPledgeCollateral(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk cid.Cid) (big.Int, error)
	// Rule[perm:read]
	StateVMCirculatingSupplyInternal(ctx context.Context, tsk cid.Cid) (chain.CirculatingSupply, error)
	// Rule[perm:read]
	StateCirculatingSupply(ctx context.Context, tsk cid.Cid) (abi.TokenAmount, error)
	// Rule[perm:read]
	StateMarketDeals(ctx context.Context, tsk cid.Cid) (map[string]types.MarketDeal, error)
	// Rule[perm:read]
	StateMinerActiveSectors(ctx context.Context, maddr address.Address, tsk cid.Cid) ([]*miner.SectorOnChainInfo, error)
	// Rule[perm:read]
	StateLookupID(ctx context.Context, addr address.Address, tsk cid.Cid) (address.Address, error)
	// Rule[perm:read]
	StateListMiners(ctx context.Context, tsk cid.Cid) ([]address.Address, error)
	// Rule[perm:read]
	StateListActors(ctx context.Context, tsk cid.Cid) ([]address.Address, error)
	// Rule[perm:read]
	StateMinerPower(ctx context.Context, addr address.Address, tsk cid.Cid) (*apitypes.MinerPower, error)
	// Rule[perm:read]
	StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tsk cid.Cid) (big.Int, error)
	// Rule[perm:read]
	StateSectorExpiration(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk cid.Cid) (*miner.SectorExpiration, error)
	// Rule[perm:read]
	StateMinerSectorCount(ctx context.Context, addr address.Address, tsk cid.Cid) (apitypes.MinerSectors, error)
	// Rule[perm:read]
	StateMarketBalance(ctx context.Context, addr address.Address, tsk cid.Cid) (apitypes.MarketBalance, error)
	// Rule[perm:read]
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk cid.Cid) (apitypes.DealCollateralBounds, error)
	// Rule[perm:read]
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk cid.Cid) (*abi.StoragePower, error)
}
