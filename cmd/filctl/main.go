package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/application-research/filclient"
	"github.com/dustin/go-humanize"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/abi"
	lblockstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/v8/actors/builtin"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/jedib0t/go-pretty/v6/table"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
)

var log = logging.Logger("filctl")

func main() {
	logging.SetLogLevel("filctl", "debug")

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)

	app := cli.NewApp()
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "data-dir",
			Value: "",
		},
	}
	app.Commands = []*cli.Command{
		{
			Name:   "retrieve",
			Action: cmdRetrieve,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "query",
					Aliases: []string{"q"},
					Usage:   "If set, only the retrieval query step will be performed",
				},
				&cli.StringFlag{
					Name:    "provider",
					Aliases: []string{"p", "miner", "m"},
					Usage:   "The provider address or peer ID",
				},
			},
		},
		{
			Name:   "find-deals",
			Action: cmdFindDeals,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "max-size",
					Usage: "Max piece size to search for (ex. '10000', '1GiB')",
				},
				&cli.UintFlag{
					Name:  "count",
					Usage: "How many results to return",
					Value: 10,
				},
			},
		},
	}
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatalf("Command failed: %v", err)
	}
}

func dataDir(ctx *cli.Context) string {
	dataDir, err := homedir.Expand("~/.filctl")
	if err != nil {
		log.Warnf("Using current working directory as data dir because home dir could not be expanded: %v", err)
		return "data"
	}
	return dataDir
}

func cmdFindDeals(ctx *cli.Context) error {
	maxSizeStr := ctx.String("max-size")
	maxSize, err := humanize.ParseBytes(maxSizeStr)
	if err != nil {
		return fmt.Errorf("failed to parse --max-size: %v", err)
	}

	count := ctx.Uint("count")

	filctl, err := New(ctx, dataDir(ctx))
	if err != nil {
		return err
	}

	ts, err := filctl.api.ChainHead(ctx.Context)
	if err != nil {
		return err
	}

	marketActor, err := filctl.api.StateGetActor(ctx.Context, builtin.StorageMarketActorAddr, ts.Key())
	if err != nil {
		return err
	}

	actorStore := store.ActorStore(ctx.Context, lblockstore.NewAPIBlockstore(filctl.api))
	marketState, err := market.Load(actorStore, marketActor)
	if err != nil {
		return err
	}

	proposals, err := marketState.Proposals()
	if err != nil {
		return err
	}

	firstUnusedDealID, err := marketState.NextID()
	if err != nil {
		return err
	}

	currCount := uint(0)

	for i := firstUnusedDealID - 1; i > 0; i-- {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if currCount == count {
			break
		}

		proposal, ok, err := proposals.Get(abi.DealID(i))
		if err != nil {
			log.Errorf("Failed to load deal ID %d: %v", i, err)
			continue
		}
		if !ok {
			continue
		}

		payloadCid, err := findPayloadCid(*proposal)
		if err != nil {
			log.Errorf("Could not extract payload CID from deal ID %d: %v", i, err)
			continue
		}

		if payloadCid.Prefix().GetCodec() == cid.Raw {
			continue
		}

		if uint64(proposal.PieceSize) > maxSize {
			continue
		}

		fmt.Printf(
			"(%s) %s %s\n",
			humanize.IBytes(uint64(proposal.PieceSize)),
			proposal.Provider,
			payloadCid,
		)

		currCount++
	}

	return nil
}

func cmdRetrieve(ctx *cli.Context) error {
	filctl, err := New(ctx, dataDir(ctx))
	if err != nil {
		return err
	}

	queryOnly := ctx.Bool("query")

	// Parse the provider handle
	var handle *filclient.MinerHandle
	addr, err := address.NewFromString(ctx.String("provider"))
	if err != nil {
		peerID, err2 := peer.IDFromString(ctx.String("provider"))
		if err2 != nil {
			return fmt.Errorf("could not parse provider string as addr (%v) or peer ID (%v)", err, err2)
		} else {
			handle = filctl.client.MinerByPeerID(peerID)
		}
	} else {
		handle = filctl.client.MinerByAddress(addr)
	}

	// Parse the payload CID
	payloadCid, err := cid.Parse(ctx.Args().First())
	if err != nil {
		return fmt.Errorf("could not parse payload CID: %v", err)
	}

	// Do retrieval query
	res, err := handle.QueryRetrievalAsk(ctx.Context, payloadCid)
	if err != nil {
		return fmt.Errorf("retrieval query failed: %v", err)
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleLight)
	t.AppendRow(table.Row{"Available", res.Status == retrievalmarket.QueryResponseAvailable})
	if res.Status == retrievalmarket.QueryResponseAvailable {
		totalPrice := types.BigAdd(
			types.BigInt(res.UnsealPrice),
			types.BigMul(res.MinPricePerByte, types.NewInt(res.Size)),
		)

		t.AppendRow(table.Row{"Retrievable", res.PieceCIDFound == retrievalmarket.QueryItemAvailable})
		t.AppendRow(table.Row{"Size", humanize.IBytes(res.Size)})
		t.AppendSeparator()
		t.AppendRow(table.Row{"Total Price", types.FIL(totalPrice)})
		t.AppendRow(table.Row{"Unseal Price", types.FIL(res.UnsealPrice)})
		t.AppendRow(table.Row{"Price Per Byte", types.FIL(res.MinPricePerByte)})
		t.AppendRow(table.Row{"Payment Interval", humanize.IBytes(res.MaxPaymentInterval)})
		t.AppendRow(table.Row{"Payment Interval Increase", humanize.IBytes(res.MaxPaymentIntervalIncrease)})
		t.AppendRow(table.Row{"Payment Address", res.PaymentAddress})
	}
	t.SetCaption(res.Message)
	fmt.Printf("%s\n", t.Render())

	// If in query-only mode, finish off now
	if queryOnly {
		return nil
	}

	transfer, err := handle.StartRetrievalTransfer(ctx.Context, payloadCid)
	if err != nil {
		return err
	}

	select {
	case <-transfer.Done():
	case <-ctx.Done():
	}

	return nil
}
