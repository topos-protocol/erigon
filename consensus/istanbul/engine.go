package istanbul

import (
	"math/big"
	"time"

	"github.com/ledgerwatch/log/v3"

	"github.com/ledgerwatch/erigon-lib/chain"
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
	VerifyBlockProposal(chain consensus.ChainHeaderReader, block *types.Block, validators consensus.ValidatorSet) (time.Duration, error)
	VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header, validators consensus.ValidatorSet) error
	// VerifyUncles(chain consensus.ChainReader, block *types.Block) error
	VerifyUncles(chain consensus.ChainReader, header *types.Header, uncles []*types.Header) error
	VerifySeal(chain consensus.ChainHeaderReader, header *types.Header, validators consensus.ValidatorSet) error
	Prepare(chain consensus.ChainHeaderReader, header *types.Header, state *state.IntraBlockState, validators consensus.ValidatorSet) error
	// Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.IntraBlockState,
	// txs []*types.Transaction, uncles []*types.Header)
	Finalize(config *chain.Config, header *types.Header, state *state.IntraBlockState,
		txs types.Transactions, uncles []*types.Header, r types.Receipts, withdrawals []*types.Withdrawal,
		chain consensus.ChainReader, syscall consensus.SystemCall, logger log.Logger,
	) (types.Transactions, types.Receipts, error)
	// FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.IntraBlockState, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error)
	FinalizeAndAssemble(chainConfig *chain.Config, header *types.Header, state *state.IntraBlockState,
		txs types.Transactions, uncles []*types.Header, receipts types.Receipts, withdrawals []*types.Withdrawal,
		chain consensus.ChainReader, syscall consensus.SystemCall, call consensus.Call, logger log.Logger,
	) (*types.Block, types.Transactions, types.Receipts, error)
	Seal(chain consensus.ChainHeaderReader, block *types.Block, validators consensus.ValidatorSet) (*types.Block, error)
	SealHash(header *types.Header) libcommon.Hash
	// CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int
	CalcDifficulty(chain consensus.ChainHeaderReader, time, parentTime uint64, parentDifficulty *big.Int, parentNumber uint64,
		parentHash, parentUncleHash libcommon.Hash, parentAuRaStep uint64) *big.Int
	CalculateRewards(config *chain.Config, header *types.Header, uncles []*types.Header, syscall consensus.SystemCall) ([]consensus.Reward, error)
	WriteVote(header *types.Header, candidate libcommon.Address, authorize bool) error
	ReadVote(header *types.Header) (candidate libcommon.Address, authorize bool, err error)
	GenerateSeal(chain consensus.ChainHeaderReader, currnt, parent *types.Header, call consensus.Call) []byte
	Initialize(config *chain.Config, chain consensus.ChainHeaderReader, header *types.Header,
		state *state.IntraBlockState, syscall consensus.SysCallCustom, logger log.Logger)
	IsServiceTransaction(sender libcommon.Address, syscall consensus.SystemCall) bool
	// Type returns underlying consensus engine
	Type() chain.ConsensusName
}
