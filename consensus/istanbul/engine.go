package istanbul

import (
	"math/big"
	"time"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/consensus"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
)

type Engine interface {
	Address() libcommon.Address
	Author(header *types.Header) (libcommon.Address, error)
	ExtractGenesisValidators(header *types.Header) ([]libcommon.Address, error)
	Signers(header *types.Header) ([]libcommon.Address, error)
	CommitHeader(header *types.Header, seals [][]byte, round *big.Int) error
	VerifyBlockProposal(chain consensus.ChainHeaderReader, block *types.Block, validators ValidatorSet) (time.Duration, error)
	VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header, validators ValidatorSet) error
	VerifyUncles(chain consensus.ChainReader, block *types.Block) error
	VerifySeal(chain consensus.ChainHeaderReader, header *types.Header, validators ValidatorSet) error
	Prepare(chain consensus.ChainHeaderReader, header *types.Header, validators ValidatorSet) error
	Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.IntraBlockState, txs []*types.Transaction, uncles []*types.Header)
	FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.IntraBlockState, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error)
	Seal(chain consensus.ChainHeaderReader, block *types.Block, validators ValidatorSet) (*types.Block, error)
	SealHash(header *types.Header) libcommon.Hash
	CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int
	WriteVote(header *types.Header, candidate libcommon.Address, authorize bool) error
	ReadVote(header *types.Header) (candidate libcommon.Address, authorize bool, err error)
}
