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

package validator

import (
	"math"
	"reflect"
	"sync"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/consensus"
)

type defaultValidator struct {
	address libcommon.Address
}

func (val *defaultValidator) Address() libcommon.Address {
	return val.address
}

func (val *defaultValidator) String() string {
	return val.Address().String()
}

// ----------------------------------------------------------------------------

type defaultSet struct {
	validators consensus.Validators
	policy     *consensus.ProposerPolicy

	proposer    consensus.Validator
	validatorMu sync.RWMutex
	selector    consensus.ProposalSelector
}

func newDefaultSet(addrs []libcommon.Address, policy *consensus.ProposerPolicy) *defaultSet {
	valSet := &defaultSet{}

	valSet.policy = policy
	// init validators
	valSet.validators = make([]consensus.Validator, len(addrs))
	for i, addr := range addrs {
		valSet.validators[i] = New(addr)
	}

	valSet.SortValidators()
	// init proposer
	if valSet.Size() > 0 {
		valSet.proposer = valSet.GetByIndex(0)
	}
	valSet.selector = roundRobinProposer
	if policy.Id == consensus.Sticky {
		valSet.selector = stickyProposer
	}

	policy.RegisterValidatorSet(valSet)

	return valSet
}

func (valSet *defaultSet) Size() int {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	return len(valSet.validators)
}

func (valSet *defaultSet) List() []consensus.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	return valSet.validators
}

func (valSet *defaultSet) GetByIndex(i uint64) consensus.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	if i < uint64(valSet.Size()) {
		return valSet.validators[i]
	}
	return nil
}

func (valSet *defaultSet) GetByAddress(addr libcommon.Address) (int, consensus.Validator) {
	for i, val := range valSet.List() {
		if addr == val.Address() {
			return i, val
		}
	}
	return -1, nil
}

func (valSet *defaultSet) GetProposer() consensus.Validator {
	return valSet.proposer
}

func (valSet *defaultSet) IsProposer(address libcommon.Address) bool {
	_, val := valSet.GetByAddress(address)
	return reflect.DeepEqual(valSet.GetProposer(), val)
}

func (valSet *defaultSet) CalcProposer(lastProposer libcommon.Address, round uint64) {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	valSet.proposer = valSet.selector(valSet, lastProposer, round)
}

// ValidatorSetSorter sorts the validators based on the configured By function
func (valSet *defaultSet) SortValidators() {
	valSet.Policy().By.Sort(valSet.validators)
}

func calcSeed(valSet consensus.ValidatorSet, proposer libcommon.Address, round uint64) uint64 {
	offset := 0
	if idx, val := valSet.GetByAddress(proposer); val != nil {
		offset = idx
	}
	return uint64(offset) + round
}

func emptyAddress(addr libcommon.Address) bool {
	return addr == libcommon.Address{}
}

func roundRobinProposer(valSet consensus.ValidatorSet, proposer libcommon.Address, round uint64) consensus.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	seed := uint64(0)
	if emptyAddress(proposer) {
		seed = round
	} else {
		seed = calcSeed(valSet, proposer, round) + 1
	}
	pick := seed % uint64(valSet.Size())
	return valSet.GetByIndex(pick)
}

func stickyProposer(valSet consensus.ValidatorSet, proposer libcommon.Address, round uint64) consensus.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	seed := uint64(0)
	if emptyAddress(proposer) {
		seed = round
	} else {
		seed = calcSeed(valSet, proposer, round)
	}
	pick := seed % uint64(valSet.Size())
	return valSet.GetByIndex(pick)
}

func (valSet *defaultSet) AddValidator(address libcommon.Address) bool {
	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()
	for _, v := range valSet.validators {
		if v.Address() == address {
			return false
		}
	}
	valSet.validators = append(valSet.validators, New(address))
	// TODO: we may not need to re-sort it again
	// sort validator
	valSet.SortValidators()
	return true
}

func (valSet *defaultSet) RemoveValidator(address libcommon.Address) bool {
	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()

	for i, v := range valSet.validators {
		if v.Address() == address {
			valSet.validators = append(valSet.validators[:i], valSet.validators[i+1:]...)
			return true
		}
	}
	return false
}

func (valSet *defaultSet) Copy() consensus.ValidatorSet {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()

	addresses := make([]libcommon.Address, 0, len(valSet.validators))
	for _, v := range valSet.validators {
		addresses = append(addresses, v.Address())
	}
	return NewSet(addresses, valSet.policy)
}

func (valSet *defaultSet) F() int { return int(math.Ceil(float64(valSet.Size())/3)) - 1 }

func (valSet *defaultSet) Policy() consensus.ProposerPolicy { return *valSet.policy }
