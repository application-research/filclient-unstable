package filclient

import (
	"context"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/ipfs/go-cid"
)

// retrieval.go - all retrieval-related functions

func (handle MinerHandle) QueryRetrievalAsk(ctx context.Context, payloadCid cid.Cid) (retrievalmarket.QueryResponse, error) {
	const protocol = "/fil/retrieval/qry/1.0.0"

	req := retrievalmarket.Query{PayloadCID: payloadCid}
	var resp retrievalmarket.QueryResponse
	if err := handle.runSingleRPC(ctx, &req, &resp, protocol); err != nil {
		return retrievalmarket.QueryResponse{}, err
	}

	return resp, nil
}
