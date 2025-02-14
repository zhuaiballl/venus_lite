package series

import (
	"context"

	"github.com/filecoin-project/go-address"
	files "github.com/ipfs/go-ipfs-files"

	"github.com/filecoin-project/venus_lite/tools/fast"
)

// SetupGenesisNode will initialize, start, configure, and issue the
// "start mining" command to the filecoin process `node`. Process `node` will
// be configured with miner `minerAddress`, and import the address of the miner
// `minerOwner`. Lastly the process `node` will start mining.
func SetupGenesisNode(ctx context.Context, node *fast.Filecoin, minerAddress address.Address, minerOwner files.File) error {
	if _, err := node.InitDaemon(ctx); err != nil {
		return err
	}

	if _, err := node.StartDaemon(ctx, true); err != nil {
		return err
	}

	if err := node.ConfigSet(ctx, "mining.minerAddress", minerAddress.String()); err != nil {
		return err
	}

	wallet, err := node.WalletImport(ctx, minerOwner)
	if err != nil {
		return err
	}
	if err := node.ConfigSet(ctx, "walletModule.defaultAddress", wallet[0].String()); err != nil {
		return err
	}

	return nil
}
