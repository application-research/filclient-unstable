package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whyrusleeping/base32"
	"golang.org/x/xerrors"
)

// Anything in tmp.go is a temporary solution, should not be here, and should be
// removed or replaced with permanent code when possible

type DiskKeyStore struct {
	path string
}

func loadOrInitPeerKey(kf string) (crypto.PrivKey, error) {
	data, err := ioutil.ReadFile(kf)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		k, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}

		data, err := crypto.MarshalPrivateKey(k)
		if err != nil {
			return nil, err
		}

		if err := ioutil.WriteFile(kf, data, 0600); err != nil {
			return nil, err
		}

		return k, nil
	}
	return crypto.UnmarshalPrivateKey(data)
}

func setupWallet(dir string) (*wallet.LocalWallet, error) {
	kstore, err := OpenOrInitKeystore(dir)
	if err != nil {
		return nil, err
	}

	wallet, err := wallet.NewWallet(kstore)
	if err != nil {
		return nil, err
	}

	addrs, err := wallet.WalletList(context.TODO())
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		_, err := wallet.WalletNew(context.TODO(), types.KTBLS)
		if err != nil {
			return nil, err
		}
	}

	return wallet, nil
}

func OpenOrInitKeystore(p string) (*DiskKeyStore, error) {
	if _, err := os.Stat(p); err == nil {
		return &DiskKeyStore{p}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.Mkdir(p, 0700); err != nil {
		return nil, err
	}

	return &DiskKeyStore{p}, nil
}

var kstrPermissionMsg = "permissions of key: '%s' are too relaxed, " +
	"required: 0600, got: %#o"

// List lists all the keys stored in the KeyStore
func (fsr *DiskKeyStore) List() ([]string, error) {

	dir, err := os.Open(fsr.path)
	if err != nil {
		return nil, xerrors.Errorf("opening dir to list keystore: %w", err)
	}
	defer dir.Close() //nolint:errcheck
	files, err := dir.Readdir(-1)
	if err != nil {
		return nil, xerrors.Errorf("reading keystore dir: %w", err)
	}
	keys := make([]string, 0, len(files))
	for _, f := range files {
		if f.Mode()&0077 != 0 {
			return nil, xerrors.Errorf(kstrPermissionMsg, f.Name(), f.Mode())
		}
		name, err := base32.RawStdEncoding.DecodeString(f.Name())
		if err != nil {
			return nil, xerrors.Errorf("decoding key: '%s': %w", f.Name(), err)
		}
		keys = append(keys, string(name))
	}
	return keys, nil
}

// Get gets a key out of keystore and returns types.KeyInfo coresponding to named key
func (fsr *DiskKeyStore) Get(name string) (types.KeyInfo, error) {

	encName := base32.RawStdEncoding.EncodeToString([]byte(name))
	keyPath := filepath.Join(fsr.path, encName)

	fstat, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		return types.KeyInfo{}, xerrors.Errorf("opening key '%s': %w", name, types.ErrKeyInfoNotFound)
	} else if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("opening key '%s': %w", name, err)
	}

	if fstat.Mode()&0077 != 0 {
		return types.KeyInfo{}, xerrors.Errorf(kstrPermissionMsg, name, fstat.Mode())
	}

	file, err := os.Open(keyPath)
	if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("opening key '%s': %w", name, err)
	}
	defer file.Close() //nolint: errcheck // read only op

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("reading key '%s': %w", name, err)
	}

	var res types.KeyInfo
	err = json.Unmarshal(data, &res)
	if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("decoding key '%s': %w", name, err)
	}

	return res, nil
}

// Put saves key info under given name
func (fsr *DiskKeyStore) Put(name string, info types.KeyInfo) error {

	encName := base32.RawStdEncoding.EncodeToString([]byte(name))
	keyPath := filepath.Join(fsr.path, encName)

	_, err := os.Stat(keyPath)
	if err == nil {
		return xerrors.Errorf("checking key before put '%s': %w", name, types.ErrKeyExists)
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("checking key before put '%s': %w", name, err)
	}

	keyData, err := json.Marshal(info)
	if err != nil {
		return xerrors.Errorf("encoding key '%s': %w", name, err)
	}

	err = ioutil.WriteFile(keyPath, keyData, 0600)
	if err != nil {
		return xerrors.Errorf("writing key '%s': %w", name, err)
	}
	return nil
}

func (fsr *DiskKeyStore) Delete(name string) error {

	encName := base32.RawStdEncoding.EncodeToString([]byte(name))
	keyPath := filepath.Join(fsr.path, encName)

	_, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		return xerrors.Errorf("checking key before delete '%s': %w", name, types.ErrKeyInfoNotFound)
	} else if err != nil {
		return xerrors.Errorf("checking key before delete '%s': %w", name, err)
	}

	err = os.Remove(keyPath)
	if err != nil {
		return xerrors.Errorf("deleting key '%s': %w", name, err)
	}
	return nil
}

func findPayloadCid(proposal market.DealProposal) (cid.Cid, error) {

	// Split the label by whitespace and attempt to parse each element
	// as a CID, finishing when one is found, otherwise moving on to the
	// next deal
	labelStr, err := proposal.Label.ToString()
	if err != nil {
		return cid.Undef, err
	}

	labelTokens := strings.Fields(labelStr)
	var payloadCid cid.Cid
	for _, token := range labelTokens {
		parsedCid, err := cid.Parse(token)
		if err != nil {
			continue
		}

		payloadCid = parsedCid
		break
	}

	if !payloadCid.Defined() {
		return cid.Undef, fmt.Errorf("could not find payload CID in label '%s'", labelStr)
	}

	return payloadCid, nil
}
