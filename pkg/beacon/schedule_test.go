package beacon

import (
	"context"
	"encoding/hex"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/pkg/config"
	"github.com/filecoin-project/venus/pkg/constants"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestSchedule_BeaconForEpoch(t *testing.T) {
	genTime, err := time.Parse("2006-01-02 15:04:05", "2020-08-25 06:00:00")
	if err != nil {
		t.Error(err)
	}
	schedule := map[abi.ChainEpoch]config.DrandEnum{0: 5, 51000: 1}
	drand, err := DrandConfigSchedule(uint64(genTime.Unix()), constants.MainNetBlockDelaySecs, schedule)
	if err != nil {
		t.Error(err)
	}
	if len(drand) != 2 {
		t.Errorf("expect %d drandserver but got %d", 2, len(drand))
	}
	beaconServer := drand.BeaconForEpoch(10)
	beaconCh := beaconServer.Entry(context.Background(), 10)
	beacon := <-beaconCh
	assert.Equal(t, beacon.Entry.Round, uint64(10))
	assert.Equal(t, hex.EncodeToString(beacon.Entry.Data), "94391b59bbb5ffd999ec336da65a8b22263cbfbcc860abf99d4567fbfad7151857a790a432f0ef3d9a702844b4bd186f17ce64cf3286e6a42f426192caf5757d9c17772bc5d199564e17ecda616f32a1e30d8e56df50fd87627072d6ad067625")

	beaconServer2 := drand.BeaconForEpoch(800000)
	beaconCh2 := beaconServer2.Entry(context.Background(), 800000)
	beacon2 := <-beaconCh2

	assert.Equal(t, beacon2.Entry.Round, uint64(800000))
	assert.Equal(t, hex.EncodeToString(beacon2.Entry.Data), "aab930d44e67aa0570f1cf74a4e8c81ff5a588be63e2f7835f88f90f2e9b7614a0004d77d8e51ac089a5c0ed9238ca7005abdbc8e0c5151dd7ad8bb7f90f89a808a5dc0685bda0571da5cedbeaf2815852713422c98a7efe01faa2d2bd1ccfef")
}
