package filclient

import (
	"context"
	"errors"
	"sync"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	"github.com/filecoin-project/go-data-transfer/network"
	"github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/lotus/api"
	"github.com/ipfs/go-datastore"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
)

// filclient.go - code related to initialization and management of the core
// FilClient struct

var log = logging.Logger("filclient")

var (
	ErrLotusError = errors.New("lotus error")
)

type Config struct {
}

type Client struct {
	host          host.Host
	api           api.Gateway
	dt            datatransfer.Manager
	dtUnsubscribe datatransfer.Unsubscribe
	bs            blockstore.Blockstore
	ds            datastore.Datastore

	// TODO(@elijaharita): this shouldn't be in the main Client struct
	retrievalTransfers   map[datatransfer.ChannelID]*RetrievalTransfer
	retrievalTransfersLk sync.Mutex
}

func New(
	ctx context.Context,
	h host.Host,
	api api.Gateway,
	addr address.Address,
	bs blockstore.Blockstore,
	ds datastore.Batching,
	opts ...Option,
) (*Client, error) {
	cfg := Config{}

	for _, opt := range opts {
		opt(&cfg)
	}

	// ctx, cancel := context.WithCancel(ctx)

	// rpc := rpcstmgr.NewRPCStateManager(api)

	// paychDS := paychmgr.NewStore(namespace.Wrap(ds, datastore.NewKey("paych")))

	dt, err := initDataTransfer(ctx, h, bs, ds)
	if err != nil {
		return nil, err
	}

	client := &Client{
		host: h,
		api:  api,
		dt:   dt,
		// dtUnsubscribe: assigned below
		bs:                 bs,
		ds:                 ds,
		retrievalTransfers: make(map[datatransfer.ChannelID]*RetrievalTransfer),
	}

	client.dtUnsubscribe = dt.SubscribeToEvents(func(
		event datatransfer.Event,
		channelState datatransfer.ChannelState,
	) {
		client.handleDataTransferRetrievalEvent(ctx, event, channelState)
	})

	return client, nil
}

func (client *Client) Close() {
	if client.dtUnsubscribe != nil {
		client.dtUnsubscribe()
	}
}

func initDataTransfer(
	ctx context.Context,
	h host.Host,
	bs blockstore.Blockstore,
	ds datastore.Batching,
) (datatransfer.Manager, error) {
	dtNetwork := network.NewFromLibp2pHost(h)
	gsNetwork := gsnet.NewFromLibp2pHost(h)
	gsExchange := gsimpl.New(ctx, gsNetwork, storeutil.LinkSystemForBlockstore(bs))
	gsTransport := graphsync.NewTransport(h.ID(), gsExchange)

	dt, err := dtimpl.NewDataTransfer(ds, dtNetwork, gsTransport)
	if err != nil {
		return nil, err
	}

	if err := dt.RegisterVoucherType(
		&requestvalidation.StorageDataTransferVoucher{},
		nil,
	); err != nil {
		return nil, err
	}

	if err := dt.RegisterVoucherType(
		&retrievalmarket.DealProposal{},
		nil,
	); err != nil {
		return nil, err
	}

	if err := dt.RegisterVoucherType(
		&retrievalmarket.DealPayment{},
		nil,
	); err != nil {
		return nil, err
	}

	if err := dt.RegisterVoucherResultType(
		&retrievalmarket.DealResponse{},
	); err != nil {
		return nil, err
	}

	if err := dt.Start(ctx); err != nil {
		return nil, err
	}

	return dt, nil
}
