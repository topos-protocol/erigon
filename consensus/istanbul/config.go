// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package istanbul

import (
	"math/big"
	"strings"

	"github.com/ledgerwatch/erigon-lib/chain"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/accounts/abi/bind"
	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/consensus"
)

type Config struct {
	RequestTimeout           uint64                    `toml:",omitempty"` // The timeout for each Istanbul round in milliseconds.
	BlockPeriod              uint64                    `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	EmptyBlockPeriod         uint64                    `toml:",omitempty"` // Default minimum difference between a block and empty block's timestamps in second
	ProposerPolicy           *consensus.ProposerPolicy `toml:",omitempty"` // The policy for proposer selection
	Epoch                    uint64                    `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
	Ceil2Nby3Block           *big.Int                  `toml:",omitempty"` // Number of confirmations required to move from one state to next [2F + 1 to Ceil(2N/3)]
	AllowedFutureBlockTime   uint64                    `toml:",omitempty"` // Max time (in seconds) from current time allowed for blocks, before they're considered future blocks
	TestQBFTBlock            *big.Int                  `toml:",omitempty"` // Fork block at which block confirmations are done using qbft consensus instead of ibft
	BeneficiaryMode          *string                   `toml:",omitempty"` // Mode for setting the beneficiary, either: list, besu, validators (beneficiary list is the list of validators)
	BlockReward              *math.HexOrDecimal256     `toml:",omitempty"` // Reward
	MiningBeneficiary        *libcommon.Address        `toml:",omitempty"` // Wallet address that benefits at every new block (besu mode)
	ValidatorContract        libcommon.Address         `toml:",omitempty"`
	Validators               []libcommon.Address       `toml:",omitempty"`
	ValidatorSelectionMode   *string                   `toml:",omitempty"`
	Client                   bind.ContractCaller       `toml:",omitempty"`
	MaxRequestTimeoutSeconds uint64                    `toml:",omitempty"`
	Transitions              []chain.Transition
}

var DefaultConfig = &Config{
	RequestTimeout:         10000,
	BlockPeriod:            5,
	EmptyBlockPeriod:       0,
	ProposerPolicy:         consensus.NewRoundRobinProposerPolicy(),
	Epoch:                  30000,
	Ceil2Nby3Block:         big.NewInt(0),
	AllowedFutureBlockTime: 0,
	TestQBFTBlock:          big.NewInt(0),
}

// QBFTBlockNumber returns the qbftBlock fork block number, returns -1 if qbftBlock is not defined
func (c Config) QBFTBlockNumber() int64 {
	if c.TestQBFTBlock == nil {
		return -1
	}
	return c.TestQBFTBlock.Int64()
}

// IsQBFTConsensusAt checks if qbft consensus is enabled for the block height identified by the given header
func (c *Config) IsQBFTConsensusAt(blockNumber *big.Int) bool {
	if c.TestQBFTBlock != nil {
		if c.TestQBFTBlock.Uint64() == 0 {
			return true
		}

		if blockNumber.Cmp(c.TestQBFTBlock) >= 0 {
			return true
		}
	}
	result := false
	if blockNumber == nil {
		blockNumber = big.NewInt(0)
	}
	c.getTransitionValue(blockNumber, func(t chain.Transition) {
		if strings.EqualFold(t.Algorithm, chain.QBFT) {
			result = true
		}
	})

	return result
}

func (c Config) GetConfig(blockNumber *big.Int) Config {
	newConfig := c

	c.getTransitionValue(blockNumber, func(transition chain.Transition) {
		if transition.RequestTimeoutSeconds != 0 {
			// RequestTimeout is on milliseconds
			newConfig.RequestTimeout = transition.RequestTimeoutSeconds * 1000
		}
		if transition.EpochLength != 0 {
			newConfig.Epoch = transition.EpochLength
		}
		if transition.BlockPeriodSeconds != 0 {
			newConfig.BlockPeriod = transition.BlockPeriodSeconds
		}
		if transition.EmptyBlockPeriodSeconds != nil {
			newConfig.EmptyBlockPeriod = *transition.EmptyBlockPeriodSeconds
		}
		if transition.BeneficiaryMode != nil {
			newConfig.BeneficiaryMode = transition.BeneficiaryMode
		}
		if transition.BlockReward != nil {
			newConfig.BlockReward = transition.BlockReward
		}
		if transition.MiningBeneficiary != nil {
			newConfig.MiningBeneficiary = transition.MiningBeneficiary
		}
		if transition.ValidatorSelectionMode != "" {
			newConfig.ValidatorSelectionMode = &transition.ValidatorSelectionMode
		}
		if transition.ValidatorContractAddress != (libcommon.Address{}) {
			newConfig.ValidatorContract = transition.ValidatorContractAddress
		}
		if len(transition.Validators) > 0 {
			newConfig.Validators = transition.Validators
		}
		if transition.MaxRequestTimeoutSeconds != nil {
			newConfig.MaxRequestTimeoutSeconds = *transition.MaxRequestTimeoutSeconds
		}
	})

	return newConfig
}

func (c Config) GetValidatorContractAddress(blockNumber *big.Int) libcommon.Address {
	validatorContractAddress := c.ValidatorContract
	c.getTransitionValue(blockNumber, func(transition chain.Transition) {
		if (transition.ValidatorContractAddress != libcommon.Address{}) {
			validatorContractAddress = transition.ValidatorContractAddress
		}
	})
	return validatorContractAddress
}

func (c Config) GetValidatorSelectionMode(blockNumber *big.Int) string {
	mode := chain.BlockHeaderMode
	if c.ValidatorSelectionMode != nil {
		mode = *c.ValidatorSelectionMode
	}
	c.getTransitionValue(blockNumber, func(transition chain.Transition) {
		if transition.ValidatorSelectionMode != "" {
			mode = transition.ValidatorSelectionMode
		}
	})
	return mode
}

func (c Config) GetValidatorsAt(blockNumber *big.Int) []libcommon.Address {
	if blockNumber.Cmp(big.NewInt(0)) == 0 && len(c.Validators) > 0 {
		return c.Validators
	}

	if blockNumber != nil && c.Transitions != nil {
		for i := 0; i < len(c.Transitions) && c.Transitions[i].Block.Cmp(blockNumber) == 0; i++ {
			return c.Transitions[i].Validators
		}
	}

	//Note! empty means we will get the valset from previous block header which contains votes, validators etc
	return []libcommon.Address{}
}

func (c Config) Get2FPlus1Enabled(blockNumber *big.Int) bool {
	twoFPlusOneEnabled := false
	c.getTransitionValue(blockNumber, func(transition chain.Transition) {
		if transition.TwoFPlusOneEnabled != nil {
			twoFPlusOneEnabled = *transition.TwoFPlusOneEnabled
		}
	})
	return twoFPlusOneEnabled
}

func (c *Config) getTransitionValue(num *big.Int, callback func(transition chain.Transition)) {
	if c != nil && num != nil && c.Transitions != nil {
		for i := 0; i < len(c.Transitions) && c.Transitions[i].Block.Cmp(num) <= 0; i++ {
			callback(c.Transitions[i])
		}
	}
}
