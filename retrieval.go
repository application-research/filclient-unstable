package filclient

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p-core/peer"
)

// retrieval.go - all retrieval-related functions

var (
	ErrUnexpectedRetrievalTransferState = errors.New("unexpected retrieval transfer state")
)

type RetrievalTransferStatus uint

const (
	// Unknown or invalid transfer state
	RetrievalTransferStatusInvalid = iota

	// Transfer was rejected up-front before it could start
	RetrievalTransferStatusRejected

	// Transfer is in progress
	RetrievalTransferStatusInProgress

	// Error occurred during transfer
	RetrievalTransferStatusErrored

	// Transfer has been stopped by the client
	RetrievalTransferStatusCancelled

	// Transfer completed successfully
	RetrievalTransferStatusCompleted
)

// Whether the retrieval status is in any of the "done states"
func (status RetrievalTransferStatus) IsDone() bool {
	return status == RetrievalTransferStatusCompleted ||
		status == RetrievalTransferStatusCancelled ||
		status == RetrievalTransferStatusErrored
}

// Operational handle for controlling and getting information about a retrieval
// transfer
type RetrievalTransfer struct {
	lk        sync.Mutex
	client    *Client
	status    RetrievalTransferStatus
	provider  peer.ID
	proposal  retrievalmarket.DealProposal
	chanID    datatransfer.ChannelID
	progress  uint64
	size      uint64
	doneChans []chan<- struct{}
}

func (transfer *RetrievalTransfer) State() RetrievalTransferStatus {
	transfer.lk.Lock()
	defer transfer.lk.Unlock()
	return transfer.status
}

// TODO(@elijaharita): the result of this function is currently inaccurate - it
// includes the total amount of bytes transferred by datatransfer (including
// vouchers, etc. on top of the total size of the blocks transferred), and also
// does not include the size of blocks already stored in the blockstore
func (transfer *RetrievalTransfer) Progress() uint64 {
	transfer.lk.Lock()
	defer transfer.lk.Unlock()
	return transfer.progress
}

func (transfer *RetrievalTransfer) Size() uint64 {
	transfer.lk.Lock()
	defer transfer.lk.Unlock()
	return transfer.size
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

// Start running a retrieval
func (handle *MinerHandle) StartRetrievalTransfer(
	ctx context.Context,
	payloadCid cid.Cid,
	options ...RetrievalOption,
) (*RetrievalTransfer, error) {
	var cfg RetrievalConfig
	for _, option := range options {
		option(&cfg)
	}
	cfg.Clean()

	// TODO(@elijaharita): allow supplying a pre-run ask result
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

	dealID, err := handle.client.nextRetrievalDealID(ctx)
	if err != nil {
		return nil, err
	}

	log.Infof("Using deal ID %d", dealID)

	proposal := retrievalmarket.DealProposal{
		PayloadCID: payloadCid,
		ID:         dealID,
		Params:     params,
	}

	peerID, err := handle.PeerID(ctx)
	if err != nil {
		return nil, err
	}

	// Open the data channel

	// NOTE: from this point to the end of the function, retrieval transfers
	// mutex will be locked so that we aren't able to look up the channel before
	// it gets registered
	handle.client.retrievalTransfersLk.Lock()
	defer handle.client.retrievalTransfersLk.Unlock()

	log.Infof("Starting data channel...")
	chanID, err := handle.client.dt.OpenPullDataChannel(
		ctx,
		peerID,
		&proposal,
		proposal.PayloadCID,
		selectorparse.CommonSelector_ExploreAllRecursively,
	)
	if err != nil {

		return nil, err
	}

	transfer := &RetrievalTransfer{
		client:   handle.client,
		status:   RetrievalTransferStatusInProgress,
		proposal: proposal,
		provider: peerID,
		chanID:   chanID,
		progress: 0,
		size:     ask.Size,
	}

	// Register with running transfers
	handle.client.retrievalTransfers[chanID] = transfer

	log.Infof("Retrieval is running")

	return transfer, nil
}

func (transfer *RetrievalTransfer) Cancel(ctx context.Context) error {
	transfer.lk.Lock()
	defer transfer.lk.Unlock()

	return transfer.cancel(ctx)
}

func (transfer *RetrievalTransfer) cancel(ctx context.Context) error {
	if err := transfer.close(ctx); err != nil {
		return err
	}

	transfer.status = RetrievalTransferStatusCancelled

	return nil
}

// Returns a channel that will close when the retrieval finishes (closes
// immediately if the retrieval is already done)
func (transfer *RetrievalTransfer) Done() <-chan struct{} {
	transfer.lk.Lock()
	defer transfer.lk.Unlock()

	return transfer.done()
}

func (transfer *RetrievalTransfer) done() <-chan struct{} {
	ch := make(chan struct{})

	if transfer.status.IsDone() {
		// If already done, signal immediately
		close(ch)
	} else {
		// Otherwise register it in the transfer info for later
		transfer.doneChans = append(transfer.doneChans, ch)
	}

	return ch
}

func (transfer *RetrievalTransfer) close(ctx context.Context) error {
	// Ensure the channel is closed
	if err := transfer.client.dt.CloseDataTransferChannel(
		ctx,
		transfer.chanID,
	); err != nil {
		return err
	}

	// Send done signals
	for _, ch := range transfer.doneChans {
		close(ch)
	}

	// Remove from active transfers
	transfer.client.retrievalTransfersLk.Lock()
	delete(transfer.client.retrievalTransfers, transfer.chanID)
	transfer.client.retrievalTransfersLk.Unlock()

	return nil
}

// Reads the next retrieval deal ID from the datastore (or initializes it as 1
// if a datastore entry doesn't exist yet), and increments the datastore entry
// afterwards
func (client *Client) nextRetrievalDealID(ctx context.Context) (retrievalmarket.DealID, error) {
	key := datastore.NewKey("/Retrieval/NextDealID")

	nextDealIDBytes, err := client.ds.Get(ctx, key)
	var nextDealID retrievalmarket.DealID
	if err != nil {
		// If there was an error and it wasn't caused by key not found, not sure
		// what to do, error out
		if !errors.Is(err, datastore.ErrNotFound) {
			return 0, err
		}

		// Otherwise if it was just key not found error, initialize deal ID as 1
		// and continue
		nextDealID = retrievalmarket.DealID(1)
	} else {
		// If loaded successfully then deserialize the deal ID bytes
		nextDealID = retrievalmarket.DealID(binary.BigEndian.Uint64(nextDealIDBytes))
	}

	// Re-serialize the deal ID + 1 and write it back to the datastore
	newNextDealIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(newNextDealIDBytes, uint64(nextDealID+1))
	if err := client.ds.Put(ctx, key, newNextDealIDBytes); err != nil {
		return retrievalmarket.DealID(0), err
	}

	return nextDealID, nil
}

func (client *Client) handleDataTransferRetrievalEvent(
	ctx context.Context,
	event datatransfer.Event,
	channelState datatransfer.ChannelState,
) {
	log.Debugf("Event code %d: %s", event.Code, datatransfer.Events[event.Code])

	client.retrievalTransfersLk.Lock()
	transfer, ok := client.retrievalTransfers[channelState.ChannelID()]
	if !ok {
		log.Errorf("Received transfer event for nonexistent channel: %s", channelState.ChannelID())
	}
	client.retrievalTransfersLk.Unlock()

	close := func() {
		transfer.lk.Lock()
		defer transfer.lk.Unlock()

		if err := transfer.close(ctx); err != nil {
			log.Errorf("Failed to close transfer with deal ID: %d", transfer.proposal.ID)
		}
	}

	switch event.Code {
	case datatransfer.NewVoucher:
		switch result := channelState.LastVoucherResult().(type) {
		case *retrievalmarket.DealResponse:
			client.handleRetrievalDealResponse(ctx, event, channelState, result)
		}
	case datatransfer.DataReceived:
		// TODO(@elijaharita): shouldn't use channelState.Received() to track this
		// value, use an intermediate blockstore layer to measure how many bytes
		// are getting actually added to the blockstore
		transfer.lk.Lock()
		transfer.progress = channelState.Received()
		transfer.lk.Unlock()
	case datatransfer.CleanupComplete:
		log.Infof("Retrieval transfer completed: %s", event.Message)
		transfer.lk.Lock()
		transfer.status = RetrievalTransferStatusCompleted
		transfer.lk.Unlock()
		close()
	}
}

func (client *Client) handleRetrievalDealResponse(
	ctx context.Context,
	event datatransfer.Event,
	channelState datatransfer.ChannelState,
	response *retrievalmarket.DealResponse,
) {
	log := log.With("channelID", channelState.ChannelID())

	close := func() {
		client.retrievalTransfersLk.Lock()
		defer client.retrievalTransfersLk.Unlock()

		transfer, ok := client.retrievalTransfers[channelState.ChannelID()]
		if !ok {
			log.Errorf("Cannot close nonexistent channel: %s", channelState.ChannelID())
		}

		transfer.lk.Lock()
		defer transfer.lk.Unlock()

		if err := transfer.close(ctx); err != nil {
			log.Errorf("Failed to close transfer with deal ID: %d", transfer.proposal.ID)
		}
	}

	switch response.Status {
	case retrievalmarket.DealStatusAccepted:
		log.Info("Retrieval transfer accepted: %s", event.Message)
	case retrievalmarket.DealStatusRejected:
		log.Error("Retrieval transfer rejected: %s", event.Message)
		close()
	case retrievalmarket.DealStatusFundsNeededUnseal:
		log.Error("UNIMPLEMENTED - Funds needed for unseal: %d", response.PaymentOwed)
		close()
	case retrievalmarket.DealStatusFundsNeeded:
		log.Error("UNIMPLEMENTED - Funds needed: %d", response.PaymentOwed)
		close()
	case retrievalmarket.DealStatusFundsNeededLastPayment:
		log.Error("UNIMPLEMENTED - Funds needed for last payment: %d", response.PaymentOwed)
		close()
	}
}
