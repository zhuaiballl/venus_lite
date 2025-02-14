package fast

import (
	"context"
	fbig "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus_lite/cmd"
	"github.com/libp2p/go-libp2p-core/peer"
	"strings"

	"github.com/filecoin-project/go-address"
	files "github.com/ipfs/go-ipfs-files"

	"github.com/filecoin-project/venus_lite/pkg/crypto"
	"github.com/filecoin-project/venus_lite/pkg/types"
)

// AddressNew runs the address new command against the filecoin process.
func (f *Filecoin) AddressNew(ctx context.Context) (address.Address, error) {
	var newAddress address.Address
	if err := f.RunCmdJSONWithStdin(ctx, nil, &newAddress, "venus", "wallet", "new"); err != nil {
		return address.Undef, err
	}
	return newAddress, nil
}

// AddressLs runs the address ls command against the filecoin process.
func (f *Filecoin) AddressLs(ctx context.Context) ([]address.Address, error) {
	// the command returns an AddressListResult
	var alr cmd.AddressLsResult
	if err := f.RunCmdJSONWithStdin(ctx, nil, &alr, "venus", "wallet", "ls"); err != nil {
		return nil, err
	}
	return alr.Addresses, nil
}

// AddressLookup runs the address lookup command against the filecoin process.
func (f *Filecoin) AddressLookup(ctx context.Context, addr address.Address) (peer.ID, error) {
	var ownerPeer peer.ID
	if err := f.RunCmdJSONWithStdin(ctx, nil, &ownerPeer, "venus", "state", "lookup", addr.String()); err != nil {
		return "", err
	}
	return ownerPeer, nil
}

// WalletBalance run the wallet balance command against the filecoin process.
func (f *Filecoin) WalletBalance(ctx context.Context, addr address.Address) (fbig.Int, error) {
	var balance fbig.Int
	if err := f.RunCmdJSONWithStdin(ctx, nil, &balance, "venus", "wallet", "balance", addr.String()); err != nil {
		return types.ZeroFIL, err
	}
	return balance, nil
}

// WalletImport run the wallet import command against the filecoin process.
func (f *Filecoin) WalletImport(ctx context.Context, file files.File) ([]address.Address, error) {
	// the command returns an AddressListResult
	var alr cmd.AddressLsResult
	if err := f.RunCmdJSONWithStdin(ctx, file, &alr, "venus", "wallet", "import"); err != nil {
		return nil, err
	}
	return alr.Addresses, nil
}

// WalletExport run the wallet export command against the filecoin process.
func (f *Filecoin) WalletExport(ctx context.Context, addrs []address.Address) ([]*crypto.KeyInfo, error) {
	// the command returns an KeyInfoListResult
	var klr cmd.WalletSerializeResult
	// we expect to interact with an array of KeyInfo(s)
	var out []*crypto.KeyInfo
	var sAddrs []string
	for _, a := range addrs {
		sAddrs = append(sAddrs, a.String())
	}

	if err := f.RunCmdJSONWithStdin(ctx, nil, &klr, "venus", "wallet", "export", strings.Join(sAddrs, " ")); err != nil {
		return nil, err
	}

	// transform the KeyInfoListResult to an array of KeyInfo(s)
	return append(out, klr.KeyInfo...), nil
}
