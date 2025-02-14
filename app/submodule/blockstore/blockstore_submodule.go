package blockstore

import (
	"context"
	"github.com/filecoin-project/venus_lite/app/client/apiface"
	"github.com/filecoin-project/venus_lite/pkg/repo"
	bstore "github.com/ipfs/go-ipfs-blockstore"
)

// BlockstoreSubmodule enhances the `Node` with local key/value storing capabilities.
// Note: at present:
// - `blockstore` is shared by chain/graphsync and piece/bitswap data
// - `cborStore` is used for chain state and shared with piece data exchange for deals at the moment.
type BlockstoreSubmodule struct { //nolint
	// blockstore is the un-networked blocks interface
	Blockstore bstore.Blockstore
}

type blockstoreRepo interface {
	Repo() repo.Repo
}

// NewBlockstoreSubmodule creates a new block store submodule.
func NewBlockstoreSubmodule(ctx context.Context, repo blockstoreRepo) (*BlockstoreSubmodule, error) {
	// set up block store
	bs := repo.Repo().Datastore()
	return &BlockstoreSubmodule{
		Blockstore: bs,
	}, nil
}

func (bsm *BlockstoreSubmodule) API() apiface.IBlockStore {
	return &blockstoreAPI{blockstore: bsm}
}

func (bsm *BlockstoreSubmodule) V0API() apiface.IBlockStore {
	return &blockstoreAPI{blockstore: bsm}
}
