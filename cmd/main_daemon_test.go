package cmd_test

import (
	"github.com/filecoin-project/venus_lite/pkg/testhelpers"
	tf "github.com/filecoin-project/venus_lite/pkg/testhelpers/testflags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"
)

func TestNoDaemonNoHang(t *testing.T) {
	tf.IntegrationTest(t)
	// Start the daemon to initialize a new repo
	d := testhelpers.NewDaemon(t).Start()

	//todo should readline for stdout for confirming rpc status
	time.Sleep(time.Second * 5)
	// rename the lock files to a safe place
	repoDir := d.RepoDir()
	require.NoError(t, os.Rename(path.Join(repoDir, "api"), path.Join(repoDir, "api.backup")))
	require.NoError(t, os.Rename(path.Join(repoDir, "repo.lock"), path.Join(repoDir, "repo.lock.backup")))

	// shut down the daemon
	d.Stop()

	// put the lock files back
	require.NoError(t, os.Rename(path.Join(repoDir, "api.backup"), path.Join(repoDir, "api")))
	require.NoError(t, os.Rename(path.Join(repoDir, "repo.lock.backup"), path.Join(repoDir, "repo.lock")))

	// run actor ls with the old repo that still has the lock file, but no running daemon
	out, _ := exec.Command(testhelpers.MustGetFilecoinBinary(), "--repodir", d.RepoDir(), "state", "list-actor").CombinedOutput()

	assert.Contains(t, string(out), "Is the daemon running?")
}
