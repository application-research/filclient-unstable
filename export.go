package filclient

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
)

// Exports the provided CID to a file
// The content must already exist in the blockstore before calling this function
func (client *Client) ExportToFile(ctx context.Context, c cid.Cid, path string, exportAsCAR bool) error {
	// Save output file
	dservOffline := merkledag.NewDAGService(blockservice.New(client.bs, offline.Exchange(client.bs)))
	dnode, err := dservOffline.Get(ctx, c)
	if err != nil {
		return err
	}

	if exportAsCAR {
		// Write file as car file

		carPath := path
		// Add .car extension, if not already specified
		if !strings.HasSuffix(path, ".car") {
			carPath += ".car"
		}

		file, err := os.Create(carPath)
		if err != nil {
			return err
		}
		car.WriteCar(ctx, dservOffline, []cid.Cid{c}, file)

		fmt.Println("Saved .car output to", path)
	} else {
		// Otherwise write file as UnixFS File
		ufsFile, err := unixfile.NewUnixfsFile(ctx, dservOffline, dnode)
		if err != nil {
			return err
		}

		if err := files.WriteTo(ufsFile, path); err != nil {
			return err
		}

		fmt.Println("Saved output to", path)
	}
	return nil
}
