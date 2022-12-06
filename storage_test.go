package filclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryStorageAsk(t *testing.T) {
	ctx := context.TODO()
	_, miner, _, fc, closer := initEnsemble(t, ctx)
	defer closer()

	ask, _, err := fc.StorageProviderByAddress(miner.ActorAddr).QueryStorageAskUnchecked(ctx)
	require.NoError(t, err)

	fmt.Printf("Storage ask: %#v\n", ask)

}
