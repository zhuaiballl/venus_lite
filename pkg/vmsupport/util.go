package vmsupport

import (
	"context"
	"fmt"

	rt5 "github.com/filecoin-project/specs-actors/v5/actors/runtime"

	"github.com/filecoin-project/venus_lite/pkg/consensusfault"
)

type NilFaultChecker struct {
}

func (n *NilFaultChecker) VerifyConsensusFault(_ context.Context, _, _, _ []byte, _ consensusfault.FaultStateView) (*rt5.ConsensusFault, error) {
	return nil, fmt.Errorf("empty chain cannot have consensus fault")
}
