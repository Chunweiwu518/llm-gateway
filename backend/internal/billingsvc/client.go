package billingsvc

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/billingpb"
)

// Client calls the billing service over gRPC (go-zero zrpc, direct endpoint).
type Client struct {
	rpc billingpb.BillingClient
}

// NewClient dials addr (host:port) without blocking, so the gateway can start before billing.
func NewClient(addr string) (*Client, error) {
	var cc zrpc.RpcClientConf
	if err := conf.FillDefault(&cc); err != nil {
		return nil, fmt.Errorf("zrpc defaults: %w", err)
	}
	logx.DisableStat() // no per-minute load-balancer stat lines in the gateway log
	cc.Endpoints = []string{addr}
	cli, err := zrpc.NewClient(cc)
	if err != nil {
		return nil, fmt.Errorf("zrpc client: %w", err)
	}
	return &Client{rpc: billingpb.NewBillingClient(cli.Conn())}, nil
}

// Settle implements relay.Settler.
func (c *Client) Settle(ctx context.Context, userID uint, reserved, actual int64) (int64, error) {
	res, err := c.rpc.Settle(ctx, &billingpb.SettleRequest{UserId: uint64(userID), Reserved: reserved, Actual: actual})
	if err != nil {
		return 0, err
	}
	return res.Charged, nil
}
