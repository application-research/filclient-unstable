package filclient

import (
	"context"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/stretchr/testify/require"
)

func TestQueryRetrievalAsk(t *testing.T) {
	ctx := context.TODO()
	client, miner, _, fc, closer := initEnsemble(t, ctx)
	defer closer()

	importRes := genDummyDeal(ctx, t, client, miner)

	ask, err := fc.MinerByAddress(miner.ActorAddr).QueryRetrievalAsk(ctx, importRes.Root)
	require.NoError(t, err)

	require.Equal(t, ask.Status, retrievalmarket.QueryResponseAvailable, "Retrieval ask: %#v", ask)

	fmt.Printf("Retrieval ask: %#v\n", ask)

}

func TestRetrievalTransfer(t *testing.T) {
	ctx := context.TODO()
	client, miner, _, fc, closer := initEnsemble(t, ctx)
	defer closer()

	importRes := genDummyDeal(ctx, t, client, miner)

	// Run the transfer
	fmt.Printf("Transferring...\n")
	transfer, err := fc.MinerByAddress(miner.ActorAddr).StartRetrievalTransfer(ctx, importRes.Root)
	require.NoError(t, err)
	<-transfer.Done()
	fmt.Printf("Finished transferring\n")

	// Verify the blocks are stored in the blockstore
	dagService := merkledag.NewDAGService(blockservice.New(fc.bs, offline.Exchange(fc.bs)))
	cidSet := cid.NewSet()
	fmt.Printf("Verifying blocks...")
	require.NoError(t, merkledag.Walk(ctx, func(ctx context.Context, c cid.Cid) ([]*format.Link, error) {
		links, err := dagService.GetLinks(ctx, c)
		require.NoErrorf(t, err, "CID missing: %s", c)
		fmt.Printf("=> %s\n", c)

		return links, nil
	}, importRes.Root, cidSet.Visit))
	fmt.Printf("Finished verifying blocks\n")

}

func genDummyDeal(ctx context.Context, t *testing.T, client *kit.TestFullNode, miner *kit.TestMiner) *api.ImportRes {
	// Create dummy deal on miner
	res, file := client.CreateImportFile(ctx, 1, int(TestSectorSize/2))
	fmt.Printf("Created import file '%s'\n", file)
	pieceInfo, err := client.ClientDealPieceCID(ctx, res.Root)
	require.NoError(t, err)
	dh := kit.NewDealHarness(t, client, miner, miner)
	dp := dh.DefaultStartDealParams()
	dp.EpochPrice.Set(big.NewInt(250_000_000))
	dp.DealStartEpoch = abi.ChainEpoch(4 << 10)
	dp.Data = &storagemarket.DataRef{
		TransferType: storagemarket.TTManual,
		Root:         res.Root,
		PieceCid:     &pieceInfo.PieceCID,
		PieceSize:    pieceInfo.PieceSize.Unpadded(),
	}
	proposalCid := dh.StartDeal(ctx, dp)

	carFileDir := t.TempDir()
	carFilePath := filepath.Join(carFileDir, "out.car")
	fmt.Printf("Generating car...\n")
	require.NoError(t, client.ClientGenCar(ctx, api.FileRef{Path: file}, carFilePath))
	fmt.Printf("Importing car...\n")
	require.NoError(t, miner.DealsImportData(ctx, *proposalCid, carFilePath))

	dh.StartSealingWaiting(ctx)
	dh.WaitDealPublished(ctx, proposalCid)

	return res
}
