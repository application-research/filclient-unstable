package filclient

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestExportFile(t *testing.T) {
	outputFilename := path.Join(t.TempDir() + "export-test")

	app := cli.NewApp()
	app.Action = func(ctx *cli.Context) error {
		client, miner, _, fc, closer := initEnsemble(t, ctx)
		defer closer()

		// Create a dummy deal
		importRes := genDummyDeal(ctx.Context, t, client, miner)

		// Transfer dummy deal into the client
		fmt.Printf("Transferring...\n")
		transfer, err := fc.MinerByAddress(miner.ActorAddr).StartRetrievalTransfer(ctx.Context, importRes.Root)
		require.NoError(t, err)
		<-transfer.Done()
		fmt.Printf("Finished transferring\n")

		// Export to file and check that it exists
		fc.ExportToFile(ctx.Context, importRes.Root, outputFilename, false)
		outFile, err := os.Stat(outputFilename)

		require.NoError(t, err)
		require.Greater(t, outFile.Size(), int64(0))

		return nil
	}

	require.NoError(t, app.Run([]string{""}))
}
