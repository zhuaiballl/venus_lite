package multisig

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	init6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/init"
	multisig6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/multisig"

	"github.com/filecoin-project/venus_lite/pkg/types/internal"
	"github.com/filecoin-project/venus_lite/pkg/types/specactors"
	init_ "github.com/filecoin-project/venus_lite/pkg/types/specactors/builtin/init"
)

type message6 struct{ message0 }

func (m message6) Create(
	signers []address.Address, threshold uint64,
	unlockStart, unlockDuration abi.ChainEpoch,
	initialAmount abi.TokenAmount,
) (*internal.Message, error) {

	lenAddrs := uint64(len(signers))

	if lenAddrs < threshold {
		return nil, xerrors.Errorf("cannot require signing of more addresses than provided for multisig")
	}

	if threshold == 0 {
		threshold = lenAddrs
	}

	if m.from == address.Undef {
		return nil, xerrors.Errorf("must provide source address")
	}

	// Set up constructor parameters for multisig
	msigParams := &multisig6.ConstructorParams{
		Signers:               signers,
		NumApprovalsThreshold: threshold,
		UnlockDuration:        unlockDuration,
		StartEpoch:            unlockStart,
	}

	enc, actErr := specactors.SerializeParams(msigParams)
	if actErr != nil {
		return nil, actErr
	}

	// new actors are created by invoking 'exec' on the init actor with the constructor params
	execParams := &init6.ExecParams{
		CodeCID:           builtin6.MultisigActorCodeID,
		ConstructorParams: enc,
	}

	enc, actErr = specactors.SerializeParams(execParams)
	if actErr != nil {
		return nil, actErr
	}

	return &internal.Message{
		To:     init_.Address,
		From:   m.from,
		Method: builtin6.MethodsInit.Exec,
		Params: enc,
		Value:  initialAmount,
	}, nil
}
