package filclient

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExportFile(t *testing.T) {
	ctx := context.TODO()
	outputFilename := path.Join(t.TempDir() + "export-test")

	client, miner, _, fc, closer := initEnsemble(t, ctx)
	defer closer()

	// Create a dummy deal
	importRes := genDummyDeal(ctx, t, client, miner)

	// Transfer dummy deal into the client
	fmt.Printf("Transferring...\n")
	transfer, err := fc.MinerByAddress(miner.ActorAddr).StartRetrievalTransfer(ctx, importRes.Root)
	require.NoError(t, err)
	<-transfer.Done()
	fmt.Printf("Finished transferring\n")

	// Export to file and check that it exists
	require.NoError(t, fc.ExportToFile(ctx, importRes.Root, outputFilename, false))
	outFile, err := os.Stat(outputFilename)

	require.NoError(t, err)
	require.Greater(t, outFile.Size(), int64(0))

}
