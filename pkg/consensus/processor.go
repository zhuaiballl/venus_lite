package consensus

import (
	"context"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus_lite/pkg/metrics/tracing"
	"github.com/filecoin-project/venus_lite/pkg/types"

	"github.com/filecoin-project/venus_lite/pkg/vm"
)

// ApplicationResult contains the result of successfully applying one message.
// ExecutionError might be set and the message can still be applied successfully.
// See ApplyMessage() for details.
type ApplicationResult struct {
	Receipt        *types.MessageReceipt
	ExecutionError error
}

// ApplyMessageResult is the result of applying a single message.
type ApplyMessageResult struct {
	ApplicationResult        // Application-level result, if error is nil.
	Failure            error // Failure to apply the message
	FailureIsPermanent bool  // Whether failure is permanent, has no chance of succeeding later.
}

// DefaultProcessor handles all block processing.
type DefaultProcessor struct {
	actors   vm.ActorCodeLoader
	syscalls vm.SyscallsImpl
}

var _ Processor = (*DefaultProcessor)(nil)

// NewDefaultProcessor creates a default processor from the given state tree and vms.
func NewDefaultProcessor(syscalls vm.SyscallsImpl) *DefaultProcessor {
	return NewConfiguredProcessor(vm.DefaultActors, syscalls)
}

// NewConfiguredProcessor creates a default processor with custom validation and rewards.
func NewConfiguredProcessor(actors vm.ActorCodeLoader, syscalls vm.SyscallsImpl) *DefaultProcessor {
	return &DefaultProcessor{
		actors:   actors,
		syscalls: syscalls,
	}
}

// ProcessTipSet computes the state transition specified by the messages in all blocks in a TipSet.
func (p *DefaultProcessor) ProcessTipSet(ctx context.Context,
	parent, ts *types.BlockHeader,
	msgs []types.BlockMessagesInfo,
	vmOption vm.VmOption,
) (cid.Cid, []types.MessageReceipt, error) {
	_, span := trace.StartSpan(ctx, "DefaultProcessor.ProcessTipSet")
	span.AddAttributes(trace.StringAttribute("blockheader", ts.String()))

	epoch := ts.Height
	var parentEpoch abi.ChainEpoch
	if parent != nil {
		parentEpoch = parent.Height
	}

	v, err := vm.NewVM(vmOption)
	if err != nil {
		return cid.Undef, nil, err
	}

	return v.ApplyTipSetMessages(msgs, ts, parentEpoch, epoch, nil)
}

//ProcessImplicitMessage compute the state of specify message but this functions skip value, gas,check
func (p *DefaultProcessor) ProcessImplicitMessage(ctx context.Context, msg *types.UnsignedMessage, vmOption vm.VmOption) (ret *vm.Ret, err error) {
	ctx, span := trace.StartSpan(ctx, "DefaultProcessor.ProcessImplicitMessage")
	span.AddAttributes(trace.StringAttribute("message", msg.String()))
	defer tracing.AddErrorEndSpan(ctx, span, &err)

	v, err := vm.NewVM(vmOption)
	if err != nil {
		return nil, err
	}
	return v.ApplyImplicitMessage(msg)
}
