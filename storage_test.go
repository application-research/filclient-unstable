package filclient

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestQueryStorageAsk(t *testing.T) {
	app := cli.NewApp()
	app.Action = func(ctx *cli.Context) error {
		_, miner, _, fc, closer := initEnsemble(t, ctx)
		defer closer()

		ask, _, err := fc.MinerByAddress(miner.ActorAddr).QueryStorageAskUnchecked(ctx.Context)
		require.NoError(t, err)

		fmt.Printf("Storage ask: %#v\n", ask)

		return nil
	}
	require.NoError(t, app.Run([]string{""}))
}
