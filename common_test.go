package filclient

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	lotusactors "github.com/filecoin-project/lotus/chain/actors"
	lotustypes "github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/itests/kit"
	lotusrepo "github.com/filecoin-project/lotus/node/repo"
	filbuiltin "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	filminer "github.com/filecoin-project/specs-actors/v6/actors/builtin/miner"
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	leveldb "github.com/ipfs/go-ds-leveldb"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-log"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// -- Setup functions

// Create and set up an ensemble with linked filclient
func initEnsemble(t *testing.T, ctx *cli.Context) (*kit.TestFullNode, *kit.TestMiner, *kit.Ensemble, *Client, func()) {

	fmt.Printf("Initializing test network...\n")

	kit.QuietMiningLogs()

	log.SetLogLevel("*", "ERROR")

	client, miner, ensemble := kit.EnsembleMinimal(t,
		kit.ThroughRPC(),        // so filclient can talk to it
		kit.MockProofs(),        // we don't care about proper sealing/proofs
		kit.SectorSize(512<<20), // 512MiB sectors
	)
	ensemble.InterconnectAll().BeginMining(50 * time.Millisecond)

	// set the *optional* on-chain multiaddr
	// the mind boggles: there is no API call for that - got to assemble your own msg
	{
		minfo, err := miner.FullNode.StateMinerInfo(ctx.Context, miner.ActorAddr, lotustypes.EmptyTSK)
		require.NoError(t, err)

		maddrNop2p, _ := multiaddr.SplitFunc(miner.ListenAddr, func(c multiaddr.Component) bool {
			return c.Protocol().Code == multiaddr.P_P2P
		})

		params, aerr := lotusactors.SerializeParams(&filminer.ChangeMultiaddrsParams{NewMultiaddrs: [][]byte{maddrNop2p.Bytes()}})
		require.NoError(t, aerr)

		_, err = miner.FullNode.MpoolPushMessage(ctx.Context, &lotustypes.Message{
			To:     miner.ActorAddr,
			From:   minfo.Worker,
			Value:  lotustypes.NewInt(0),
			Method: filbuiltin.MethodsMiner.ChangeMultiaddrs,
			Params: params,
		}, nil)
		require.NoError(t, err)
	}

	fmt.Printf("Test client fullnode running on %s\n", client.ListenAddr)
	os.Setenv("FULLNODE_API_INFO", client.ListenAddr.String())

	client.WaitTillChain(ctx.Context, kit.BlockMinedBy(miner.ActorAddr))

	// FilClient initialization
	fmt.Printf("Initializing filclient...\n")

	// give filc the pre-funded wallet from the client
	ki, err := client.WalletExport(ctx.Context, client.DefaultKey.Address)
	require.NoError(t, err)
	lr, err := lotusrepo.NewMemory(nil).Lock(lotusrepo.Wallet)
	require.NoError(t, err)
	ks, err := lr.KeyStore()
	require.NoError(t, err)
	wallet, err := wallet.NewWallet(ks)
	require.NoError(t, err)
	_, err = wallet.WalletImport(ctx.Context, ki)
	require.NoError(t, err)

	h, err := ensemble.Mocknet().GenPeer()
	if err != nil {
		t.Fatalf("Could not gen p2p peer: %v", err)
	}
	ensemble.Mocknet().LinkAll()
	api, closer := initAPI(t, ctx)
	bs := initBlockstore(t)
	ds := initDatastore(t)
	fc, err := New(
		ctx.Context,
		h,
		api,
		client.DefaultKey.Address,
		bs,
		ds,
		t.TempDir(),
	) // WithWallet(wallet)
	if err != nil {
		t.Fatalf("Could not initialize FilClient: %v", err)
	}

	// Wait for actor address to appear on chain
	time.Sleep(time.Millisecond * 500)

	fmt.Printf("Ready\n")

	return client, miner, ensemble, fc, closer
}

func initAPI(t *testing.T, ctx *cli.Context) (api.Gateway, jsonrpc.ClientCloser) {
	api, closer, err := lcli.GetGatewayAPI(ctx)
	if err != nil {
		t.Fatalf("Could not initialize Lotus API gateway: %v", err)
	}

	return api, closer
}

func initBlockstore(t *testing.T) blockstore.Blockstore {
	parseShardFunc, err := flatfs.ParseShardFunc("/repo/flatfs/shard/v1/next-to-last/3")
	if err != nil {
		t.Fatalf("Blockstore parse shard func failed: %v", err)
	}

	ds, err := flatfs.CreateOrOpen(filepath.Join(t.TempDir(), "blockstore"), parseShardFunc, false)
	if err != nil {
		t.Fatalf("Could not initialize blockstore: %v", err)
	}

	bs := blockstore.NewBlockstoreNoPrefix(ds)

	return bs
}

func initDatastore(t *testing.T) datastore.Batching {
	ds, err := leveldb.NewDatastore(filepath.Join(t.TempDir(), "datastore"), nil)
	if err != nil {
		t.Fatalf("Could not initialize datastore: %v", err)
	}

	return ds
}
