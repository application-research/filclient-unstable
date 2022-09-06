package filclient

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/host"
)

// filclient.go - code related to initialization and management of the core
// FilClient struct

var (
	ErrLotusError = errors.New("lotus error")
)

type Config struct {
}

type Client struct {
	host host.Host
	api  api.Gateway
}

func New(
	ctx context.Context,
	h host.Host,
	api api.Gateway,
	addr address.Address,
	bs blockstore.Blockstore,
	ds datastore.Batching,
	dataDir string,
	opts ...Option,
) (*Client, error) {
	cfg := Config{}

	for _, opt := range opts {
		opt(&cfg)
	}

	// ctx, cancel := context.WithCancel(ctx)

	// rpc := rpcstmgr.NewRPCStateManager(api)

	// paychDS := paychmgr.NewStore(namespace.Wrap(ds, datastore.NewKey("paych")))

	return &Client{
		host: h,
		api:  api,
	}, nil
}
