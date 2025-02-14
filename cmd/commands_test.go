package cmd_test

import (
	"testing"

	"github.com/filecoin-project/venus_lite/app/node/test"
	"github.com/filecoin-project/venus_lite/fixtures/fortest"
	th "github.com/filecoin-project/venus_lite/pkg/testhelpers"
)

// create a basic new TestDaemon, with a miner and the KeyInfo it needs to sign
// tickets and blocks. This does not set a DefaultAddress in the Wallet; in this
// case, Init generates a new address in the wallet and sets it to
// the default address.
//nolint
func makeTestDaemonWithMinerAndStart(t *testing.T) *th.TestDaemon {
	daemon := th.NewDaemon(
		t,
		th.KeyFile(fortest.KeyFilePaths()[0]),
	).Start()
	return daemon
}

func buildWithMiner(t *testing.T, builder *test.NodeBuilder) {
	// bundle together common init options for node test state
	cs := test.FixtureChainSeed(t)
	builder.WithGenesisInit(cs.GenesisInitFunc)
	//builder.WithConfig(cs.MinerConfigOpt(0))
	builder.WithInitOpt(cs.MinerInitOpt(0))
}
