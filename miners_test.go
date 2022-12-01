package filclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinerVersion(t *testing.T) {
	ctx := context.TODO()

	_, miner, _, fc, closer := initEnsemble(t, ctx)
	defer closer()

	version, err := fc.MinerByAddress(miner.ActorAddr).Version(ctx)
	require.NoError(t, err)
	fmt.Printf("Found miner version: %s\n", version)

}

func TestMinerAddressToPeerID(t *testing.T) {
	ctx := context.TODO()
	_, miner, _, fc, closer := initEnsemble(t, ctx)
	defer closer()

	minerPeerID, err := fc.MinerByAddress(miner.ActorAddr).PeerID(ctx)
	require.NoError(t, err)
	fmt.Printf("Mapped miner address %s to peer ID %s\n", miner.ActorAddr, minerPeerID)
}

// TODO(@elijaharita): peer id -> address mapping is not functional yet

// func TestMinerPeerIDToAddress(t *testing.T) {
// 	app := cli.NewApp()
// 	app.Action = func(ctx *cli.Context) error {
// 		_, miner, _, fc, closer := initEnsemble(t, ctx)
// 		defer closer()

// 		minerPeerID, err := miner.ID(ctx.Context)
// 		require.NoError(t, err)

// 		minerAddr, err := fc.MinerByPeerID(minerPeerID).Address(ctx.Context)
// 		require.NoError(t, err)
// 		fmt.Printf("Mapped miner peer ID %s to address %s\n", minerPeerID, minerAddr)

// 		return nil
// 	}
// 	require.NoError(t, app.Run([]string{""}))
// }
