package chain

import (
	"context"
	"github.com/filecoin-project/venus_lite/pkg/types"
	"time"
)

// WrapHeadChangeCoalescer wraps a ReorgNotifee with a head change coalescer.
// minDelay is the minimum coalesce delay; when a head change is first received, the coalescer will
//  wait for that long to coalesce more head changes.
// maxDelay is the maximum coalesce delay; the coalescer will not delay delivery of a head change
//  more than that.
// mergeInterval is the interval that triggers additional coalesce delay; if the last head change was
//  within the merge interval when the coalesce timer fires, then the coalesce time is extended
//  by min delay and up to max delay total.
func WrapHeadChangeCoalescer(fn ReorgNotifee, minDelay, maxDelay, mergeInterval time.Duration) ReorgNotifee {
	c := NewHeadChangeCoalescer(fn, minDelay, maxDelay, mergeInterval)
	return c.HeadChange
}

// HeadChangeCoalescer is a stateful reorg notifee which coalesces incoming head changes
// with pending head changes to reduce state computations from head change notifications.
type HeadChangeCoalescer struct {
	notify ReorgNotifee

	ctx    context.Context
	cancel func()

	eventq chan headChange

	revert *types.BlockHeader
	apply  *types.BlockHeader
}

type headChange struct {
	revert, apply *types.BlockHeader
}

// NewHeadChangeCoalescer creates a HeadChangeCoalescer.
func NewHeadChangeCoalescer(fn ReorgNotifee, minDelay, maxDelay, mergeInterval time.Duration) *HeadChangeCoalescer {
	ctx, cancel := context.WithCancel(context.Background())
	c := &HeadChangeCoalescer{
		notify: fn,
		ctx:    ctx,
		cancel: cancel,
		eventq: make(chan headChange),
	}

	go c.background(minDelay, maxDelay, mergeInterval)

	return c
}

// HeadChange is the ReorgNotifee callback for the stateful coalescer; it receives an incoming
// head change and schedules dispatch of a coalesced head change in the background.
func (c *HeadChangeCoalescer) HeadChange(revert, apply *types.BlockHeader) error {
	select {
	case c.eventq <- headChange{revert: revert, apply: apply}:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// Close closes the coalescer and cancels the background dispatch goroutine.
// Any further notification will result in an error.
func (c *HeadChangeCoalescer) Close() error {
	select {
	case <-c.ctx.Done():
	default:
		c.cancel()
	}

	return nil
}

// Implementation details

func (c *HeadChangeCoalescer) background(minDelay, maxDelay, mergeInterval time.Duration) {
	var timerC <-chan time.Time
	var first, last time.Time

	for {
		select {
		case evt := <-c.eventq:
			c.coalesce(evt.revert, evt.apply)

			now := time.Now()
			last = now
			if first.IsZero() {
				first = now
			}

			if timerC == nil {
				timerC = time.After(minDelay)
			}

		case now := <-timerC:
			sinceFirst := now.Sub(first)
			sinceLast := now.Sub(last)

			if sinceLast < mergeInterval && sinceFirst < maxDelay {
				// coalesce some more
				maxWait := maxDelay - sinceFirst
				wait := minDelay
				if maxWait < wait {
					wait = maxWait
				}

				timerC = time.After(wait)
			} else {
				// dispatch
				c.dispatch()

				first = time.Time{}
				last = time.Time{}
				timerC = nil
			}

		case <-c.ctx.Done():
			if c.revert != nil || c.apply != nil {
				c.dispatch()
			}
			return
		}
	}
}

func (c *HeadChangeCoalescer) coalesce(revert, apply *types.BlockHeader) {
	// newly reverted tipsets cancel out with pending applys.
	// similarly, newly applied tipsets cancel out with pending reverts.

	// pending tipsets
	pendRevert := make(map[string]struct{}, 1)
	//for _, ts := range c.revert {
	pendRevert[c.revert.Cid().String()] = struct{}{}
	//}

	pendApply := make(map[string]struct{}, 1)
	//for _, ts := range c.apply {
	pendApply[c.apply.Cid().String()] = struct{}{}
	//}

	// incoming tipsets
	reverting := make(map[string]struct{}, 1)
	//for _, ts := range revert {
	reverting[revert.Cid().String()] = struct{}{}
	//}

	applying := make(map[string]struct{}, 1)
	//for _, ts := range apply {
	applying[apply.Cid().String()] = struct{}{}
	//}

	// coalesced revert set
	// - pending reverts are cancelled by incoming applys
	// - incoming reverts are cancelled by pending applys
	newRevert := make([]*types.BlockHeader, 0, 2)
	//for _, ts := range c.revert {
	_, cancel := applying[c.revert.Cid().String()]
	if cancel {
		//continue
	}

	newRevert = append(newRevert, c.revert)
	//}

	//for _, ts := range revert {
	_, cancel = pendApply[revert.Cid().String()]
	if cancel {
		//continue
	}

	newRevert = append(newRevert, revert)
	//}

	// coalesced apply set
	// - pending applys are cancelled by incoming reverts
	// - incoming applys are cancelled by pending reverts
	newApply := make([]*types.BlockHeader, 0, 2)
	//for _, ts := range c.apply {
	_, cancel = reverting[c.apply.Cid().String()]
	if cancel {
		//continue
	}

	newApply = append(newApply, c.apply)
	//}

	//for _, ts := range apply {
	_, cancel = pendRevert[apply.Cid().String()]
	if cancel {
		//continue
	}

	newApply = append(newApply, apply)
	//}

	// commit the coalesced sets
	c.revert = newRevert[0]
	c.apply = newApply[0]
}

func (c *HeadChangeCoalescer) dispatch() {
	err := c.notify(c.revert, c.apply)
	if err != nil {
		log.Errorf("error dispatching coalesced head change notification: %s", err)
	}

	c.revert = nil
	c.apply = nil
}
