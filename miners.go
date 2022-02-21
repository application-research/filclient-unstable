package filclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	core "github.com/libp2p/go-libp2p-core"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
)

var (
	ErrMinerConnectionFailed = errors.New("miner connection failed")
)

func (fc *FilClient) MinerVersion(ctx context.Context, miner address.Address) (string, error) {
	peer, err := fc.connectToMiner(ctx, miner)
	if err != nil {
		return "", err
	}

	version, err := fc.host.Peerstore().Get(peer, "AgentVersion")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLotusError, err)
	}

	return version.(string), nil
}

// BEHAVIOR CHANGE - no longer errors on invalid multiaddr if at least one valid
// multiaddr exists
func (fc *FilClient) connectToMiner(ctx context.Context, miner address.Address) (core.PeerID, error) {
	info, err := fc.api.StateMinerInfo(ctx, miner, types.EmptyTSK)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLotusError, err)
	}

	if info.PeerId == nil {
		return "", fmt.Errorf("%w: miner info has no peer ID set", ErrLotusError)
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

	if err := fc.host.Connect(ctx, peer.AddrInfo{
		ID:    *info.PeerId,
		Addrs: multiaddrs,
	}); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMinerConnectionFailed, err)
	}

	return *info.PeerId, nil
}
