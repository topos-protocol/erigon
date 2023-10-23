package stateless

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/turbo/trie"
)

var (
	witnessesBucket = "witnesses"
)

type WitnessDBWriter struct {
	storage     kv.RwTx
	statsWriter *csv.Writer
}

func NewWitnessDBWriter(storage kv.RwTx, statsWriter *csv.Writer) (*WitnessDBWriter, error) {
	err := statsWriter.Write([]string{
		"blockNum", "maxTrieSize", "witnessesSize",
	})
	if err != nil {
		return nil, err
	}
	return &WitnessDBWriter{storage, statsWriter}, nil
}

func (db *WitnessDBWriter) MustUpsert(blockNumber uint64, maxTrieSize uint32, resolveWitnesses []*trie.Witness) {
	key := deriveDbKey(blockNumber, maxTrieSize)

	var buf bytes.Buffer

	for i, witness := range resolveWitnesses {
		if _, err := witness.WriteInto(&buf); err != nil {
			panic(fmt.Errorf("error while writing witness to a buffer: %w", err))
		}
		if i < len(resolveWitnesses)-1 {
			buf.WriteByte(byte(trie.OpNewTrie))
		}
	}

	bytes := buf.Bytes()

	batch := db.storage

	err := batch.Put(witnessesBucket, common.CopyBytes(key), common.CopyBytes(bytes))

	if err != nil {
		panic(fmt.Errorf("error while upserting witness: %w", err))
	}

	err = db.statsWriter.Write([]string{
		fmt.Sprintf("%v", blockNumber),
		fmt.Sprintf("%v", maxTrieSize),
		fmt.Sprintf("%v", len(bytes)),
	})

	if err != nil {
		panic(fmt.Errorf("error while writing stats: %w", err))
	}

	db.statsWriter.Flush()
}

type WitnessDBReader struct {
	getter kv.Getter
}

func NewWitnessDBReader(getter kv.Getter) *WitnessDBReader {
	return &WitnessDBReader{getter}
}

func (db *WitnessDBReader) GetWitnessesForBlock(blockNumber uint64, maxTrieSize uint32) ([]byte, error) {
	key := deriveDbKey(blockNumber, maxTrieSize)
	return db.getter.GetOne(witnessesBucket, key)
}

func deriveDbKey(blockNumber uint64, maxTrieSize uint32) []byte {
	buffer := make([]byte, 8+4)

	binary.LittleEndian.PutUint64(buffer[:], blockNumber)
	binary.LittleEndian.PutUint32(buffer[8:], maxTrieSize)

	return buffer
}
