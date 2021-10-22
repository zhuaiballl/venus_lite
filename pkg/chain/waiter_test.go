package chain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/venus_lite/pkg/constants"
	_ "github.com/filecoin-project/venus_lite/pkg/crypto/bls"
	_ "github.com/filecoin-project/venus_lite/pkg/crypto/secp"
	tf "github.com/filecoin-project/venus_lite/pkg/testhelpers/testflags"
	"github.com/filecoin-project/venus_lite/pkg/types"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
)

var mockSigner, _ = types.NewMockSignersAndKeyInfo(10)

var newSignedMessage = types.NewSignedMessageForTestGetter(mockSigner)

func setupTest(t *testing.T) (cbor.IpldStore, *Store, *MessageStore, *Waiter) {
	builder := NewBuilder(t, address.Undef)
	return builder.cstore, builder.store, builder.mstore, NewWaiter(builder.store, builder.mstore, builder.bs, builder.cstore)
}

func TestWaitRespectsContextCancel(t *testing.T) {
	tf.UnitTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	_, _, _, waiter := setupTest(t)

	var err error
	var chainMessage *ChainMessage
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		chainMessage, err = waiter.Wait(ctx, newSignedMessage(0), constants.DefaultConfidence, constants.DefaultMessageWaitLookback, true)
	}()

	cancel()

	select {
	case <-doneCh:
		fmt.Println(err)
		//assert.Error(t, err)
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Wait should have returned when context was canceled")
	}
	assert.Nil(t, chainMessage)
}
