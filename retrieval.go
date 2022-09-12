package filclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/ipfs/go-cid"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p-core/peer"
)

// retrieval.go - all retrieval-related functions

var (
	ErrUnexpectedRetrievalTransferState = errors.New("unexpected retrieval transfer state")
)

type RetrievalTransferState uint

const (
	RetrievalTransferStateInvalid = iota
	RetrievalTransferStateProposed
	RetrievalTransferStateInProgress
	RetrievalTransferStateStopped
	RetrievalTransferStateDone
)

type RetrievalTransfer struct {
	state    RetrievalTransferState
	provider peer.ID
	proposal retrievalmarket.DealProposal
	dtchan   datatransfer.ChannelID
}

func (transfer *RetrievalTransfer) State() RetrievalTransferState {
	return transfer.state
}

func (handle *MinerHandle) QueryRetrievalAsk(ctx context.Context, payloadCid cid.Cid) (retrievalmarket.QueryResponse, error) {
	const protocol = "/fil/retrieval/qry/1.0.0"

	req := retrievalmarket.Query{PayloadCID: payloadCid}
	var resp retrievalmarket.QueryResponse
	if err := handle.runSingleRPC(ctx, &req, &resp, protocol); err != nil {
		return retrievalmarket.QueryResponse{}, err
	}

	return resp, nil
}

// WIP
//
// Sets up params for a retrieval deal which can then be started with
// StartRetrievalTransfer()
func (handle *MinerHandle) InitRetrievalTransfer(
	ctx context.Context,
	payloadCid cid.Cid,
	options ...RetrievalOption,
) (*RetrievalTransfer, error) {
	var cfg RetrievalConfig
	for _, option := range options {
		option(&cfg)
	}
	cfg.Clean()

	ask, err := handle.QueryRetrievalAsk(ctx, payloadCid)
	if err != nil {
		return nil, err
	}

	// Create proposal

	params, err := retrievalmarket.NewParamsV1(
		ask.MinPricePerByte,
		ask.MaxPaymentInterval,
		ask.MaxPaymentIntervalIncrease,
		cfg.selector,
		nil,
		ask.UnsealPrice,
	)
	if err != nil {
		return nil, err
	}

	proposal := retrievalmarket.DealProposal{
		PayloadCID: payloadCid,
		ID:         retrievalmarket.DealID(rand.Int63n(1000000) + 100000),
		Params:     params,
	}

	return &RetrievalTransfer{
		state:    RetrievalTransferStateProposed,
		proposal: proposal,
	}, nil
}

// WIP
func (handle *MinerHandle) StartRetrievalTransfer(
	ctx context.Context,
	transfer *RetrievalTransfer,
) error {
	// Transfer must be in the "proposed" state to start
	if transfer.state != RetrievalTransferStateProposed {
		return fmt.Errorf("%w: %v", ErrUnexpectedRetrievalTransferState, transfer.state)
	}

	dtchan, err := handle.client.dt.OpenPullDataChannel(
		ctx,
		transfer.provider,
		&transfer.proposal,
		transfer.proposal.PayloadCID,
		selectorparse.CommonSelector_ExploreAllRecursively,
	)
	if err != nil {
		return err
	}

	transfer.dtchan = dtchan
	handle.client.retrievalTransfers[dtchan] = transfer

	return nil
}

// WIP
func (handle *MinerHandle) StopRetrievalTransfer(
	ctx context.Context,
	transfer *RetrievalTransfer,
) error {
	if err := handle.client.dt.CloseDataTransferChannel(
		ctx,
		transfer.dtchan,
	); err != nil {
		return err
	}

	return nil
}
