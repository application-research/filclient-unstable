package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/application-research/filclient-unstable"
	"github.com/dustin/go-humanize"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	lblockstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
)

var log = logging.Logger("filctl")

func main() {
	logging.SetLogLevel("filctl", "info")

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
				&cli.StringFlag{
					Name:     "output",
					Aliases:  []string{"o", "output"},
					Usage:    "The output file location",
					Required: false,
				},
				&cli.BoolFlag{
					Name:    "car",
					Aliases: []string{"c", "car"},
					Usage:   "If set, will export result as CAR, otherwise will export as UnixFS",
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
					Value: "64GiB",
				},
				&cli.UintFlag{
					Name:  "count",
					Usage: "How many results to return",
					Value: 10,
				},
				&cli.UintFlag{
					Name:  "per-provider-count",
					Usage: "The maximum amount of results to return from a single provider",
					Value: 1,
				},
				&cli.UintFlag{
					Name:  "offset",
					Usage: "How many deals back to start (useful if you want to find deals from longer ago)",
				},
				&cli.BoolFlag{
					Name:    "yes",
					Aliases: []string{"y"},
					Usage:   "Assume yes for default-yes prompts (or no default-no prompts)",
				},
			},
		},
		{
			Name:   "clear-blockstore",
			Action: cmdClearBlockstore,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "yes",
					Aliases: []string{"y"},
					Usage:   "Assume yes for default-yes prompts (or no default-no prompts)",
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

	fmt.Printf("Searching for deals...\n")

	count := ctx.Uint("count")

	perProviderCount := ctx.Uint("per-provider-count")

	offset := ctx.Uint("offset")

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

	dealID := firstUnusedDealID - 1 - abi.DealID(offset)
	var lk sync.Mutex

	scanCount := uint(0)
	currCount := uint(0)
	currProviderCounts := make(map[address.Address]uint)

	var wg sync.WaitGroup
	for threadNum := 0; threadNum < 8; threadNum++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}

				if currCount == count {
					break
				}

				fmt.Printf("Scanned %d deals\t\t\r", scanCount)
				scanCount++

				lk.Lock()
				copiedDealID := dealID
				dealID--
				lk.Unlock()

				proposal, ok, err := proposals.Get(copiedDealID)
				if err != nil {
					// Only print the error if it wasn't a context error
					if ctx.Err() == nil {
						log.Errorf("Failed to load deal ID %d: %v", copiedDealID, err)
					}
					continue
				}
				if !ok {
					continue
				}

				if currProviderCounts[proposal.Provider] == perProviderCount {
					continue
				}

				payloadCid, err := findPayloadCid(*proposal)
				if err != nil {
					log.Debugf("Could not extract payload CID from deal ID %d: %v", copiedDealID, err)
					continue
				}

				if payloadCid.Prefix().GetCodec() == cid.Raw {
					continue
				}

				if uint64(proposal.PieceSize) > maxSize {
					continue
				}

				lk.Lock()

				fmt.Printf(
					"(%s) %s %s\n",
					humanize.IBytes(uint64(proposal.PieceSize)),
					proposal.Provider,
					payloadCid,
				)

				currCount++
				currProviderCounts[proposal.Provider]++
				lk.Unlock()
			}
		}()
	}
	wg.Wait()

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
		peerID, err2 := peer.Decode(ctx.String("provider"))
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

	path := ctx.String("output")

	if path != "" && !fs.ValidPath(path) {
		return fmt.Errorf("invalid output location '%s'", path)
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

	if res.Status != retrievalmarket.QueryResponseAvailable {
		return nil
	}

	// If in query-only mode, finish off now
	if queryOnly {
		return nil
	}

	// Allow user to confirm the retrieval
	if !prompt(ctx, "Continue with retrieval?", true) {
		return nil
	}

	transfer, err := handle.StartRetrievalTransfer(ctx.Context, payloadCid)
	if err != nil {
		return err
	}

	success := false

	for range time.Tick(time.Millisecond * 100) {
		if ctx.Err() != nil {
			break
		}

		if transfer.State().IsDone() {
			success = true
			break
		}

		fmt.Fprintf(
			os.Stderr,
			"\r%s / %s (%d / %d)",
			humanize.IBytes(transfer.Progress()),
			humanize.IBytes(transfer.Size()),
			transfer.Progress(),
			transfer.Size(),
		)
	}

	fmt.Fprintf(os.Stdout, "\n")

	if path != "" && success {
		filctl.client.ExportToFile(ctx.Context, payloadCid, path, ctx.Bool("car"))
	}

	return nil
}

func cmdClearBlockstore(ctx *cli.Context) error {
	blockstorePath := filepath.Join(dataDir(ctx), "blockstore")

	if !prompt(ctx, fmt.Sprintf("Delete blockstore? (at path '%s')", blockstorePath), true) {
		return nil
	}

	if err := os.RemoveAll(blockstorePath); err != nil {
		return err
	}

	fmt.Printf("Deleted blockstore\n")

	return nil
}

func prompt(ctx *cli.Context, question string, defaultYes bool) bool {
	if defaultYes {
		fmt.Printf("%s [Y/n] ", question)
	} else {
		fmt.Printf("%s [y/N] ", question)
	}

	if ctx.Bool("yes") {
		fmt.Printf("\n")
		return defaultYes
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if scanner.Text() == "" {
		return defaultYes
	}
	return strings.ToLower(scanner.Text()) == "y"
}
