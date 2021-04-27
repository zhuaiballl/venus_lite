package client

import (
	"context"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"net/http"
)

func getVenusClientInfo(api, token string) (string, http.Header, error) {
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+token)
	apima, err := multiaddr.NewMultiaddr(api)
	if err != nil {
		return "", nil, err
	}

	_, addr, err := manet.DialArgs(apima)
	if err != nil {
		return "", nil, err
	}

	addr = "ws://" + addr + "/rpc/v0"
	return addr, headers, nil
}

//NewFullNode 用于构造一个全节点访问客户端，api可以从~/.venus/api文件中获取，在本地jwt模式下从～/.venus/token读取，
//在中心授权的模式下，则从venus-auth服务中获取。
func NewFullNode(ctx context.Context, api, token string) (FullNode, jsonrpc.ClientCloser, error) {
	addr, headers, err := getVenusClientInfo(api, token)
	if err != nil {
		return FullNode{}, nil, err
	}
	node := FullNode{}
	closer, err := jsonrpc.NewClient(ctx, addr, "Filecoin", &node, headers)
	if err != nil {
		return FullNode{}, nil, err
	}
	return node, closer, nil
}
