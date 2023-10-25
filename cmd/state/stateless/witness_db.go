package stateless

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"fmt"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/turbo/trie"
)

type WitnessDBWriter struct {
	storage     kv.RwDB
	statsWriter *csv.Writer
}

func NewWitnessDBWriter(storage kv.RwDB, statsWriter *csv.Writer) (*WitnessDBWriter, error) {
	err := statsWriter.Write([]string{
		"blockNum", "maxTrieSize", "witnessesSize",
	})
	if err != nil {
		return nil, err
	}
	return &WitnessDBWriter{storage, statsWriter}, nil
}

func (db *WitnessDBWriter) MustUpsertOneWitness(blockNumber uint64, witness *trie.Witness) {
	k := make([]byte, 8)

	binary.LittleEndian.PutUint64(k[:], blockNumber)

	var buf bytes.Buffer
	_, err := witness.WriteInto(&buf)
	if err != nil {
		panic(fmt.Sprintf("error extracting witness for block %d: %v\n", blockNumber, err))
	}

	bytes := buf.Bytes()

	tx, err := db.storage.BeginRw(context.Background())
	if err != nil {
		panic(fmt.Errorf("error opening tx: %w", err))
	}

	defer tx.Rollback()

	err = tx.Put(kv.Witnesses, k, common.CopyBytes(bytes))

	tx.Commit()

	if err != nil {
		panic(fmt.Errorf("error while upserting witness: %w", err))
	}
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

	tx, err := db.storage.BeginRw(context.Background())
	if err != nil {
		panic(fmt.Errorf("error opening tx: %w", err))
	}

	defer tx.Rollback()

	err = tx.Put(kv.Witnesses, common.CopyBytes(key), common.CopyBytes(bytes))

	tx.Commit()

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
	db kv.RwDB
}

func NewWitnessDBReader(db kv.RwDB) *WitnessDBReader {
	return &WitnessDBReader{db}
}

func (db *WitnessDBReader) GetWitnessesForBlock(blockNumber uint64, maxTrieSize uint32) ([]byte, error) {
	key := deriveDbKey(blockNumber, maxTrieSize)

	tx, err := db.db.BeginRo(context.Background())
	if err != nil {
		panic(fmt.Errorf("error opening tx: %w", err))
	}

	defer tx.Rollback()

	return tx.GetOne(kv.Witnesses, key)
}

func deriveDbKey(blockNumber uint64, maxTrieSize uint32) []byte {
	buffer := make([]byte, 8+4)

	binary.LittleEndian.PutUint64(buffer[:], blockNumber)
	binary.LittleEndian.PutUint32(buffer[8:], maxTrieSize)

	return buffer
}
