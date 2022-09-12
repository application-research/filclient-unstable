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
	"github.com/filecoin-project/lotus/chain/types"
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

	// If not in query-only mode, do the retrieval
	if !queryOnly {
		log.Fatalf("Only queries are supported")
	}

	return nil
}

func init() {

}