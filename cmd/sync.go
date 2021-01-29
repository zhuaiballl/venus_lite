// Package commands implements the command to print the blockchain.
package cmd

import (
	"bytes"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/app/submodule/syncer"
	syncTypes "github.com/filecoin-project/venus/pkg/chainsync/types"
	"github.com/ipfs/go-cid"
	cmds "github.com/ipfs/go-ipfs-cmds"
	"strconv"
	"time"

	"github.com/filecoin-project/venus/app/node"
)

var syncCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Inspect the sync",
	},
	Subcommands: map[string]*cmds.Command{
		"status":         storeStatusCmd,
		"history":        historyCmd,
		"wait":           waitCmd,
		"set-concurrent": setConcurrent,
	},
}
var setConcurrent = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "set concurrent of sync thread",
	},
	Options: []cmds.Option{
		cmds.Int64Option("concurrent", "coucurrent of sync thread"),
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("concurrent", true, false, "coucurrent of sync thread"),
	},
	Run: func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		concurrent, err := strconv.Atoi(req.Arguments[0])
		if err != nil {
			return cmds.ClientError("invalid number")
		}
		env.(*node.Env).SyncerAPI.SetConcurrent(int64(concurrent))
		return nil
	},
}

var storeStatusCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show status of chain sync operation.",
	},
	Run: func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		tracker := env.(*node.Env).SyncerAPI.SyncerTracker()
		targets := tracker.Buckets()
		w := bytes.NewBufferString("")
		writer := NewSilentWriter(w)
		var inSyncing []*syncTypes.Target
		var waitTarget []*syncTypes.Target

		for _, t := range targets {
			if t.State == syncTypes.StateInSyncing {
				inSyncing = append(inSyncing, t)
			} else {
				waitTarget = append(waitTarget, t)
			}
		}
		count := 1
		writer.Println("Syncing:")
		for _, t := range inSyncing {
			writer.Println("SyncTarget:", strconv.Itoa(count))
			writer.Println("\tBase:", t.Base.EnsureHeight(), t.Base.Key().String())
			writer.Println("\tTarget:", t.Head.EnsureHeight(), t.Head.Key().String())

			if t.Current != nil {
				writer.Println("\tCurrent:", t.Current.EnsureHeight(), t.Current.Key().String())
			} else {
				writer.Println("\tCurrent:")
			}

			writer.Println("\tStatus:", t.State.String())
			writer.Println("\tErr:", t.Err)
			writer.Println()
			count++
		}

		writer.Println("Waiting:")
		for _, t := range waitTarget {
			writer.Println("SyncTarget:", strconv.Itoa(count))
			writer.Println("\tBase:", t.Base.EnsureHeight(), t.Base.Key().String())
			writer.Println("\tTarget:", t.Head.EnsureHeight(), t.Head.Key().String())

			if t.Current != nil {
				writer.Println("\tCurrent:", t.Current.EnsureHeight(), t.Current.Key().String())
			} else {
				writer.Println("\tCurrent:")
			}

			writer.Println("\tStatus:", t.State.String())
			writer.Println("\tErr:", t.Err)
			writer.Println()
			count++
		}

		if err := re.Emit(w); err != nil {
			return err
		}
		return nil
	},
}

var historyCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show history of chain sync.",
	},
	Run: func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		tracker := env.(*node.Env).SyncerAPI.SyncerTracker()
		w := bytes.NewBufferString("")
		writer := NewSilentWriter(w)

		writer.Println("History:")
		history := tracker.History()
		count := 1
		for _, t := range history {
			writer.Println("SyncTarget:", strconv.Itoa(count))
			writer.Println("\tBase:", t.Base.EnsureHeight(), t.Base.Key().String())

			writer.Println("\tTarget:", t.Head.EnsureHeight(), t.Head.Key().String())

			if t.Current != nil {
				writer.Println("\tCurrent:", t.Current.EnsureHeight(), t.Current.Key().String())
			} else {
				writer.Println("\tCurrent:")
			}
			writer.Println("\tTime:", t.End.Sub(t.Start).Milliseconds())
			writer.Println("\tStatus:", t.State.String())
			writer.Println("\tErr:", t.Err)
			writer.Println()
			count++
		}

		if err := re.Emit(w); err != nil {
			return err
		}
		return nil
	},
}

var waitCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "wait for chain sync completed",
	},
	Options: []cmds.Option{
		cmds.BoolOption("watch").WithDefault(false),
	},
	Run: func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		api := env.(*node.Env).SyncerAPI
		chainApi := env.(*node.Env).ChainAPI
		watch := req.Options["watch"].(bool)
		blockDelay := env.(*node.Env).InspectorAPI.Config().NetworkParams.BlockDelay
		tick := time.Second / 4

		lastLines := 0
		ticker := time.NewTicker(tick)
		defer ticker.Stop()

		samples := 8
		i := 0
		var firstApp, app, lastApp uint64

		state, err := api.SyncState(req.Context)
		if err != nil {
			return err
		}

		firstApp = state.VMApplied

		for {
			state, err := api.SyncState(req.Context)
			if err != nil {
				return err
			}

			if len(state.ActiveSyncs) == 0 {
				time.Sleep(time.Second)
				continue
			}

			head, err := chainApi.ChainHead(req.Context)
			if err != nil {
				return err
			}

			working := -1
			for i, ss := range state.ActiveSyncs {
				switch ss.Stage {
				case syncer.StageSyncComplete:
				default:
					working = i
				case syncer.StageIdle:
					// not complete, not actively working
				}
			}

			if working == -1 {
				working = len(state.ActiveSyncs) - 1
			}

			ss := state.ActiveSyncs[working]
			workerID := ss.WorkerID

			var baseHeight abi.ChainEpoch
			var target []cid.Cid
			var theight abi.ChainEpoch
			var heightDiff int64

			if ss.Base != nil {
				baseHeight = ss.Base.EnsureHeight()
				heightDiff = int64(ss.Base.EnsureHeight())
			}
			if ss.Target != nil {
				target = ss.Target.Key().Cids()
				theight = ss.Target.EnsureHeight()
				heightDiff = int64(ss.Target.EnsureHeight()) - heightDiff
			} else {
				heightDiff = 0
			}

			for i := 0; i < lastLines; i++ {
				printOneString(re, "\r\x1b[2K\x1b[A")
			}

			printOneString(re, fmt.Sprintf("Worker: %d; Base: %d; Target: %d (diff: %d)\n", workerID, baseHeight, theight, heightDiff))
			printOneString(re, fmt.Sprintf("State: %s; Current Epoch: %d; Todo: %d\n", ss.Stage, ss.Height, theight-ss.Height))
			lastLines = 2

			if i%samples == 0 {
				lastApp = app
				app = state.VMApplied - firstApp
			}
			if i > 0 {
				printOneString(re, fmt.Sprintf("Validated %d messages (%d per second)\n", state.VMApplied-firstApp, (app-lastApp)*uint64(time.Second/tick)/uint64(samples)))
				lastLines++
			}

			_ = target // todo: maybe print? (creates a bunch of line wrapping issues with most tipsets)

			if !watch && time.Now().Unix()-int64(head.MinTimestamp()) < int64(blockDelay) {
				printOneString(re, fmt.Sprintf("\nDone!"))
				return nil
			}

			select {
			case <-req.Context.Done():
				printOneString(re, fmt.Sprintf("\nExit by user"))
				return nil
			case <-ticker.C:
			}

			i++
		}

		return nil
	},
}
