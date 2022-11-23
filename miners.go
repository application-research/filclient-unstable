package filclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// miners.go - code for managing information about individual miners

var (
	ErrMinerConnectionFailed = errors.New("miner connection failed")
	ErrMinerStreamFailed     = errors.New("stream failed")
	ErrCBORWriteFailed       = errors.New("CBOR write failed")
	ErrCBORReadFailed        = errors.New("CBOR read failed")
)

// A miner handle contains all the functions used to interact with the miner,
// and facilitates mapping between addresses and peer IDs
type MinerHandle struct {
	// WARNING: may be uninitialized - use .Address()
	addr address.Address
	// WARNING: may be uninitialized - use .PeerID()
	peerID peer.ID
	client *Client
}

func (client *Client) MinerByAddress(addr address.Address) *MinerHandle {
	return &MinerHandle{
		addr:   addr,
		client: client,
	}
}

func (client *Client) MinerByPeerID(peerID peer.ID) *MinerHandle {
	return &MinerHandle{
		peerID: peerID,
		client: client,
	}
}

// Returns the stored address of the provider, or errors if it is not known
//
// In the future, this function may be able to derive the address from the peer
// ID
func (handle *MinerHandle) Address(ctx context.Context) (address.Address, error) {
	if handle.addr == address.Undef {
		return address.Undef, fmt.Errorf("peer ID to address mapping is not yet implemented")
	}

	return handle.addr, nil
}

// Returns the peer ID of the provider, looking it up on chain using the address
// if not already stored
func (handle *MinerHandle) PeerID(ctx context.Context) (peer.ID, error) {
	info, err := handle.client.api.StateMinerInfo(ctx, handle.addr, types.EmptyTSK)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLotusError, err)
	}

	if info.PeerId == nil {
		return "", fmt.Errorf("%w: miner info has no peer ID set on chain", ErrLotusError)
	}

	handle.peerID = *info.PeerId

	return handle.peerID, nil
}

// Looks up the version string of the miner
func (handle *MinerHandle) Version(ctx context.Context) (string, error) {
	peer, err := handle.Connect(ctx)
	if err != nil {
		return "", err
	}

	version, err := handle.client.host.Peerstore().Get(peer, "AgentVersion")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLotusError, err)
	}

	return version.(string), nil
}

// Opens a P2P stream to the miner
func (handle *MinerHandle) stream(ctx context.Context, protocols ...protocol.ID) (network.Stream, error) {
	peer, err := handle.Connect(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := handle.client.host.NewStream(ctx, peer, protocols...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMinerStreamFailed, err)
	}

	return stream, nil
}

// Sends a single RPC request, and puts the response into resp - handy but not
// ideal for multiple requests
//
// TODO(@elijaharita): generics
func (handle *MinerHandle) runSingleRPC(ctx context.Context, req interface{}, resp interface{}, protocols ...protocol.ID) error {
	stream, err := handle.stream(ctx, protocols...)
	if err != nil {
		return err
	}
	defer stream.Close()

	dline, ok := ctx.Deadline()
	if ok {
		stream.SetDeadline(dline)
		defer stream.SetDeadline(time.Time{})
	}

	if err := cborutil.WriteCborRPC(stream, req); err != nil {
		return fmt.Errorf("%w: %v", ErrCBORWriteFailed, err)
	}

	if err := cborutil.ReadCborRPC(stream, resp); err != nil {
		return fmt.Errorf("%w: %v", ErrCBORReadFailed, err)
	}

	return nil
}

// Makes sure that the miner is connected
//
// BEHAVIOR CHANGE - no longer errors on invalid multiaddr if at least one valid
// multiaddr exists
func (handle *MinerHandle) Connect(ctx context.Context) (peer.ID, error) {
	info, err := handle.client.api.StateMinerInfo(ctx, handle.addr, types.EmptyTSK)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLotusError, err)
	}

	// We have to find the peer ID here anyway, so populate it
	handle.peerID = *info.PeerId

	if info.PeerId == nil {
		return "", fmt.Errorf("%w: miner info has no peer ID set on chain", ErrLotusError)
	}

	// Parse the multiaddr bytes
	var multiaddrs []multiaddr.Multiaddr
	hadInvalid := false
	for _, addrBytes := range info.Multiaddrs {
		multiaddr, err := multiaddr.NewMultiaddrBytes(addrBytes)
		if err != nil {
			// If an address failed to parse, keep going but make note
			hadInvalid = true
			continue
		}

		multiaddrs = append(multiaddrs, multiaddr)
	}
	fmt.Printf("Connecting to %v (%s)\n", multiaddrs, handle.peerID)

	// FIXME - lotus-client-proper falls back on the DHT when it has a peerid but no multiaddr
	// filc should do the same
	if len(multiaddrs) == 0 {
		// If there were addresses and they were all invalid (hadInvalid marked
		// true and multiaddrs length 0), specifically mention that
		if hadInvalid {
			return "", fmt.Errorf("%w: miner info has only invalid multiaddrs", ErrMinerConnectionFailed)
		}

		// Otherwise, just mention no multiaddrs available
		return "", fmt.Errorf("%w: miner info has no multiaddrs", ErrMinerConnectionFailed)
	}

	if err := handle.client.host.Connect(ctx, peer.AddrInfo{
		ID:    *info.PeerId,
		Addrs: multiaddrs,
	}); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMinerConnectionFailed, err)
	}

	return *info.PeerId, nil
}
