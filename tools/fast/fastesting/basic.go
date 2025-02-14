package fastesting

import (
	"context"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-ipfs-files"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/venus_lite/pkg/testhelpers"
	"github.com/filecoin-project/venus_lite/tools/fast"
	"github.com/filecoin-project/venus_lite/tools/fast/environment"
	"github.com/filecoin-project/venus_lite/tools/fast/series"
	localplugin "github.com/filecoin-project/venus_lite/tools/iptb-plugins/filecoin/local"
)

// TestEnvironment provides common setup for writing tests using FAST
type TestEnvironment struct {
	environment.Environment

	t   *testing.T
	ctx context.Context

	pluginName string
	pluginOpts map[string]string

	fastenvOpts fast.FilecoinOpts

	GenesisMiner *fast.Filecoin
}

// NewTestEnvironment creates a TestEnvironment with a basic setup for writing tests using the FAST library.
func NewTestEnvironment(ctx context.Context, t *testing.T, fastenvOpts fast.FilecoinOpts) (context.Context, *TestEnvironment) {

	// Create a directory for the test using the test name (mostly for FAST)
	// Replace the forward slash as tempdir can't handle them
	dir, err := ioutil.TempDir("", strings.Replace(t.Name(), "/", ".", -1))
	require.NoError(t, err)

	// Create an environment that includes a genesis block with 1MM FIL
	env, err := environment.NewMemoryGenesis(big.NewInt(1000000), dir)
	require.NoError(t, err)

	defer func() {
		dumpEnvOutputOnFail(t, env.Processes())
	}()

	// Setup options for nodes.
	options := make(map[string]string)
	options[localplugin.AttrLogJSON] = "1"                                        // Enable JSON logs
	options[localplugin.AttrLogLevel] = "5"                                       // Set log level to Debug
	options[localplugin.AttrFilecoinBinary] = testhelpers.MustGetFilecoinBinary() // Get the filecoin binary

	genesisURI := env.GenesisCar()
	genesisMiner, err := env.GenesisMiner()
	require.NoError(t, err)

	fastenvOpts.InitOpts = append([]fast.ProcessInitOption{fast.POGenesisFile(genesisURI)}, fastenvOpts.InitOpts...)

	// Setup the first node which is used to help coordinate the other nodes by providing
	// funds, mining for the network, etc
	genesis, err := env.NewProcess(ctx, localplugin.PluginName, options, fastenvOpts)
	require.NoError(t, err)

	err = series.SetupGenesisNode(ctx, genesis, genesisMiner.Address, files.NewReaderFile(genesisMiner.Owner))
	require.NoError(t, err)

	// Define a MessageWait function which will bet set on the context to provide
	// a way to wait for a message to appear on the mining queue
	//var waitForMpool series.MpoolWaitFunc = func() {
	//	_, err := genesis.MpoolLs(ctx, fast.AOWaitForCount(1))
	//	require.NoError(t, err)
	//}

	//ctx = series.SetCtxWaitForMpool(ctx, waitForMpool)
	ctx = series.SetCtxSleepDelay(ctx, time.Second)

	return ctx, &TestEnvironment{
		Environment:  env,
		t:            t,
		ctx:          ctx,
		pluginName:   localplugin.PluginName,
		pluginOpts:   options,
		fastenvOpts:  fastenvOpts,
		GenesisMiner: genesis,
	}
}

// RequireNewNode builds a new node for the environment
func (env *TestEnvironment) RequireNewNode() *fast.Filecoin {
	p, err := env.NewProcess(env.ctx, env.pluginName, env.pluginOpts, env.fastenvOpts)
	require.NoError(env.t, err)

	return p
}

// RequireNewNodeStarted builds a new node using RequireNewNode, then initializes
// and starts it
func (env *TestEnvironment) RequireNewNodeStarted() *fast.Filecoin {
	p := env.RequireNewNode()

	err := series.InitAndStart(env.ctx, p)
	require.NoError(env.t, err)

	return p
}

// RequireNewNodeConnected builds a new node using RequireNewNodeStarted, then
// connect it to the environment GenesisMiner node
func (env *TestEnvironment) RequireNewNodeConnected() *fast.Filecoin {
	p := env.RequireNewNodeStarted()

	err := series.Connect(env.ctx, env.GenesisMiner, p)
	require.NoError(env.t, err)

	return p
}

// RequireNewNodeWithFunds builds a new node using RequireNewNodeStarted, then
// sends it funds from the environment GenesisMiner node
func (env *TestEnvironment) RequireNewNodeWithFunds(funds int) *fast.Filecoin {
	p := env.RequireNewNodeConnected()

	err := series.SendFilecoinDefaults(env.ctx, env.GenesisMiner, p, funds)
	require.NoError(env.t, err)

	return p
}

// Teardown stops all of the nodes and cleans up the environment. If the test failed,
// it will also print the last output of each process by calling `DumpLastOutput`.
// Output is logged using the Log method on the testing.T
func (env *TestEnvironment) Teardown(ctx context.Context) error {
	env.DumpEnvOutputOnFail()
	return env.Environment.Teardown(ctx)
}

// DumpEnvOutputOnFail calls `DumpLastOutput for each process if the test failed.
func (env *TestEnvironment) DumpEnvOutputOnFail() {
	dumpEnvOutputOnFail(env.t, env.Processes())
}

// RunAsyncMiner unset MiningOnce for conflict
func (env *TestEnvironment) RunAsyncMiner() context.Context {
	var miningOnce series.MiningOnceFunc = func() {}
	var mpoolWait series.MpoolWaitFunc = func() {}
	env.ctx = series.SetCtxMiningOnce(env.ctx, miningOnce)
	env.ctx = series.SetCtxWaitForMpool(env.ctx, mpoolWait)
	return env.ctx
}

// helper to dump the output using the t.Log method.
func dumpEnvOutputOnFail(t *testing.T, procs []*fast.Filecoin) {
	if t.Failed() {
		w := newLogWriter(t)
		for _, node := range procs {
			node.DumpLastOutput(w)
		}
		require.NoError(t, w.Close())
	}
}
