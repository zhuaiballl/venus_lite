package discovery

import (
	"github.com/filecoin-project/venus_lite/app/client/apiface"
)

var _ apiface.IDiscovery = &discoveryAPI{}

type discoveryAPI struct { //nolint
	discovery *DiscoverySubmodule
}
