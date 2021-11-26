package internal

import (
	"github.com/ipfs/go-cid"
)

// FullBlock carries a newBlock header and the message and receipt collections
// referenced from the header.
type FullBlock struct {
	Header       *BlockHeader
	BLSMessages  []*UnsignedMessage
	SECPMessages []*SignedMessage
}

// Cid returns the FullBlock's header's Cid
func (fb *FullBlock) Cid() cid.Cid {
	return fb.Header.Cid()
}

func ReverseFullBlock(chain []*FullBlock) {
	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(chain)/2 - 1; i >= 0; i-- {
		opp := len(chain) - 1 - i
		chain[i], chain[opp] = chain[opp], chain[i]
	}
}
