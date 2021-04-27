package multisig

import (
	"github.com/filecoin-project/venus/app/submodule/chain"
	"github.com/filecoin-project/venus/app/submodule/mpool"
	chain2 "github.com/filecoin-project/venus/pkg/chain"
)

type MultiSigSubmodule struct { //nolint
	state chain.IChain
	mpool mpool.IMessagePool
	store *chain2.Store
}

// MessagingSubmodule enhances the `Node` with multisig capabilities.
func NewMultiSigSubmodule(chainState chain.IChain, msgPool mpool.IMessagePool, store *chain2.Store) *MultiSigSubmodule {
	return &MultiSigSubmodule{state: chainState, mpool: msgPool, store: store}
}

//API create a new multisig implement
func (sb *MultiSigSubmodule) API() IMultiSig {
	return newMultiSig(sb)
}
