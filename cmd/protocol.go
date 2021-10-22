package cmd

import (
	"github.com/filecoin-project/venus_lite/app/node"
	"github.com/filecoin-project/venus_lite/app/submodule/apitypes"
	cmds "github.com/ipfs/go-ipfs-cmds"
)

var protocolCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show protocol parameter details",
	},
	Run: func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		params, err := env.(*node.Env).ChainAPI.ProtocolParameters(req.Context)
		if err != nil {
			return err
		}
		return re.Emit(params)
	},
	Type: apitypes.ProtocolParams{},
}
