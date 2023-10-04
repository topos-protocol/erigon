// Trace types for sending proof information to a zk prover as defined in https://github.com/0xPolygonZero/proof-protocol-decoder.
package types

import (
	"github.com/holiman/uint256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
)

type ContractCodeUsage struct {
	Read  libcommon.Hash
	Write []byte
}

type TxnTrace struct {
	Balance        *uint256.Int
	Nonce          *uint64
	StorageRead    []libcommon.Hash
	StorageWritten map[libcommon.Hash]uint256.Int
	CodeUsage      ContractCodeUsage
}

type TxnMeta struct {
	ByteCode           []byte
	NewTxnTrieNode     []byte
	NewReceiptTrieNode []byte
	GasUsed            uint64
	Bloom              Bloom
}

type TxnInfo struct {
	Traces map[libcommon.Address]TxnTrace
	Meta   TxnMeta
}

type BlockUsedCodeHashes []libcommon.Hash

type TriePreImage []byte

type StorageTriesPreImage map[libcommon.Address]TriePreImage

type BlockTrace struct {
	StateTrie    TriePreImage
	StorageTries StorageTriesPreImage
	ContractCode BlockUsedCodeHashes
	TxnInfo      TxnInfo
}
