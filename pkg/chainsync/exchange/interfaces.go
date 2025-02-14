package exchange

import (
	"context"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"github.com/ipfs/go-cid"

	"github.com/libp2p/go-libp2p-core/peer"
)

// Server is the responder side of the ChainExchange protocol. It accepts
// requests from clients and services them by returning the requested
// chain data.
type Server interface {
	Register()
}

// Client is the requesting side of the ChainExchange protocol. It acts as
// a proxy for other components to request chain data from peers. It is chiefly
// used by the Syncer.
type Client interface {
	// GetBlocks fetches block headers from the network, from the provided
	// tipset *backwards*, returning as many tipsets as the count parameter,
	// or less.
	//GetBlocks(ctx context.Context, tsk types.TipSetKey, count int) ([]*types.TipSet, error)
	GetBlocks(ctx context.Context, blockID cid.Cid, count int) ([]*types.BlockHeader, error)
	// GetChainMessages fetches messages from the network, starting from the first provided tipset
	// and returning messages from as many tipsets as requested or less.
	//GetChainMessages(ctx context.Context, tipsets []*types.TipSet) ([]*CompactedMessages, error)

	// GetFullBlock fetches a full block from a given peer. If successful,
	// the fetched object contains block headers and all messages in full form.
	//GetFullTipSet(ctx context.Context, peer []peer.ID, tsk types.TipSetKey) (*types.FullTipSet, error)
	GetFullBlock(ctx context.Context, peers []peer.ID, blockID cid.Cid) (*types.FullBlock, error)
	//GetChainMessages fetches all the messages in headers
	GetChainMessages(ctx context.Context, headers []*types.BlockHeader) ([]*types.FullBlock, error)
	// AddPeer adds a peer to the pool of peers that the Client requests
	// data from.
	AddPeer(peer peer.ID)

	// RemovePeer removes a peer from the pool of peers that the Client
	// requests data from.
	RemovePeer(peer peer.ID)
}
