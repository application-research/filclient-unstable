package main

import (
	"os"
	"path"

	"github.com/application-research/filclient"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	flatfs "github.com/ipfs/go-ds-flatfs"
	leveldb "github.com/ipfs/go-ds-leveldb"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p"
	"github.com/urfave/cli/v2"
)

type Filctl struct {
	client    *filclient.Client
	api       api.Gateway
	apiCloser jsonrpc.ClientCloser
}

func New(ctx *cli.Context, dataDir string) (*Filctl, error) {
	host, err := libp2p.New()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	ds, err := leveldb.NewDatastore(path.Join(dataDir, "datastore"), nil)
	if err != nil {
		return nil, err
	}

	bsDS, err := flatfs.CreateOrOpen(
		path.Join(dataDir, "blockstore"),
		flatfs.NextToLast(3),
		false,
	)
	if err != nil {
		return nil, err
	}

	bs := blockstore.NewBlockstoreNoPrefix(bsDS)

	api, apiCloser, err := lcli.GetGatewayAPI(ctx)
	if err != nil {
		return nil, err
	}

	wallet, err := setupWallet(path.Join(dataDir, "wallet"))
	if err != nil {
		return nil, err
	}

	addr, err := wallet.GetDefault()
	if err != nil {
		return nil, err
	}

	client, err := filclient.New(ctx.Context, host, api, addr, bs, ds)
	if err != nil {
		return nil, err
	}

	return &Filctl{
		client:    client,
		api:       api,
		apiCloser: apiCloser,
	}, nil
}
