package state

import (
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/common"
)

type PreimageWriter struct {
	db            kv.GetPut
	savePreimages bool
}

func (pw *PreimageWriter) SetSavePreimages(save bool) {
	pw.savePreimages = save
}

func (pw *PreimageWriter) HashAddress(address libcommon.Address, save bool) (libcommon.Hash, error) {
	addrHash, err := common.HashData(address[:])
	if err != nil {
		return libcommon.Hash{}, err
	}
	return addrHash, pw.savePreimage(save, addrHash[:], address[:])
}

func (pw *PreimageWriter) HashKey(key *libcommon.Hash, save bool) (libcommon.Hash, error) {
	keyHash, err := common.HashData(key[:])
	if err != nil {
		return libcommon.Hash{}, err
	}
	return keyHash, pw.savePreimage(save, keyHash[:], key[:])
}

func (pw *PreimageWriter) savePreimage(save bool, hash []byte, preimage []byte) error {
	if !save || !pw.savePreimages {
		return nil
	}
	// Following check is to minimise the overwriting the same value of preimage
	// in the database, which would cause extra write churn
	if p, _ := pw.db.GetOne(kv.PreimagePrefix, hash); p != nil {
		return nil
	}
	return pw.db.Put(kv.PreimagePrefix, hash, preimage)
}
