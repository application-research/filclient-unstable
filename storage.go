package filclient

import (
	"context"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-state-types/crypto"
)

// TODO
// func (handle *MinerHandle) QueryStorageAsk(ctx context.Context) (storagemarket.StorageAsk, error) {}

// Queries a storage ask, returning the signature without validating it
func (handle MinerHandle) QueryStorageAskUnchecked(ctx context.Context) (storagemarket.StorageAsk, crypto.Signature, error) {
	const protocol = "/fil/storage/ask/1.1.0"

	req := network.AskRequest{Miner: handle.addr}
	var resp network.AskResponse
	if err := handle.runSingleRPC(ctx, &req, &resp, protocol); err != nil {
		return storagemarket.StorageAsk{}, crypto.Signature{}, err
	}

	return *resp.Ask.Ask, *resp.Ask.Signature, nil
}

// TODO
// // Checks the validity of the ask against its signature, returning nil if ok, or
// // erroring if invalid
// func CheckStorageAsk(ask storagemarket.StorageAsk, signature crypto.Signature) error {}
