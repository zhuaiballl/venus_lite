package events

import (
	"context"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
)

type messageCache struct {
	api IEvent

	blockMsgLk    sync.Mutex
	blockMsgCache *lru.ARCCache
}

func newMessageCache(api IEvent) *messageCache {
	blsMsgCache, _ := lru.NewARC(500)

	return &messageCache{
		api:           api,
		blockMsgCache: blsMsgCache,
	}
}

func (c *messageCache) ChainGetBlockMessages(ctx context.Context, blkCid cid.Cid) (*apitypes.BlockMessages, error) {
	c.blockMsgLk.Lock()
	defer c.blockMsgLk.Unlock()

	msgsI, ok := c.blockMsgCache.Get(blkCid)
	var err error
	if !ok {
		msgsI, err = c.api.ChainGetBlockMessages(ctx, blkCid)
		if err != nil {
			return nil, err
		}
		c.blockMsgCache.Add(blkCid, msgsI)
	}
	return msgsI.(*apitypes.BlockMessages), nil
}
