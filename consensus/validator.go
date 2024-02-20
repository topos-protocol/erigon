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

package consensus

import (
	"bytes"
	"sort"
	"strings"
	"sync"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/naoina/toml"
)

type ProposerPolicyId uint64

const (
	RoundRobin ProposerPolicyId = iota
	Sticky
)

// ProposerPolicy represents the Validator Proposer Policy
type ProposerPolicy struct {
	Id         ProposerPolicyId    // Could be RoundRobin or Sticky
	By         ValidatorSortByFunc // func that defines how the ValidatorSet should be sorted
	registry   []ValidatorSet      // Holds the ValidatorSet for a given block height
	registryMU *sync.Mutex         // Mutex to lock access to changes to Registry
}

// NewRoundRobinProposerPolicy returns a RoundRobin ProposerPolicy with ValidatorSortByString as default sort function
func NewRoundRobinProposerPolicy() *ProposerPolicy {
	return NewProposerPolicy(RoundRobin)
}

// NewStickyProposerPolicy return a Sticky ProposerPolicy with ValidatorSortByString as default sort function
func NewStickyProposerPolicy() *ProposerPolicy {
	return NewProposerPolicy(Sticky)
}

func NewProposerPolicy(id ProposerPolicyId) *ProposerPolicy {
	return NewProposerPolicyByIdAndSortFunc(id, ValidatorSortByString())
}

func NewProposerPolicyByIdAndSortFunc(id ProposerPolicyId, by ValidatorSortByFunc) *ProposerPolicy {
	return &ProposerPolicy{Id: id, By: by, registryMU: new(sync.Mutex)}
}

type proposerPolicyToml struct {
	Id ProposerPolicyId
}

func (p *ProposerPolicy) MarshalTOML() (interface{}, error) {
	if p == nil {
		return nil, nil
	}
	pp := &proposerPolicyToml{Id: p.Id}
	data, err := toml.Marshal(pp)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (p *ProposerPolicy) UnmarshalTOML(decode func(interface{}) error) error {
	var innerToml string
	err := decode(&innerToml)
	if err != nil {
		return err
	}
	var pp proposerPolicyToml
	err = toml.Unmarshal([]byte(innerToml), &pp)
	if err != nil {
		return err
	}
	p.Id = pp.Id
	p.By = ValidatorSortByString()
	return nil
}

// Use sets the ValidatorSortByFunc for the given ProposerPolicy and sorts the validatorSets according to it
func (p *ProposerPolicy) Use(v ValidatorSortByFunc) {
	p.By = v

	for _, validatorSet := range p.registry {
		validatorSet.SortValidators()
	}
}

// RegisterValidatorSet stores the given ValidatorSet in the policy registry
func (p *ProposerPolicy) RegisterValidatorSet(valSet ValidatorSet) {
	p.registryMU.Lock()
	defer p.registryMU.Unlock()

	if len(p.registry) == 0 {
		p.registry = []ValidatorSet{valSet}
	} else {
		p.registry = append(p.registry, valSet)
	}
}

// ClearRegistry removes any ValidatorSet from the ProposerPolicy registry
func (p *ProposerPolicy) ClearRegistry() {
	p.registryMU.Lock()
	defer p.registryMU.Unlock()

	p.registry = nil
}

type Validator interface {
	// Address returns address
	Address() libcommon.Address

	// String representation of Validator
	String() string
}

// ----------------------------------------------------------------------------

type Validators []Validator

func (vs validatorSorter) Len() int {
	return len(vs.validators)
}

func (vs validatorSorter) Swap(i, j int) {
	vs.validators[i], vs.validators[j] = vs.validators[j], vs.validators[i]
}

func (vs validatorSorter) Less(i, j int) bool {
	return vs.by(vs.validators[i], vs.validators[j])
}

type validatorSorter struct {
	validators Validators
	by         ValidatorSortByFunc
}

type ValidatorSortByFunc func(v1 Validator, v2 Validator) bool

func ValidatorSortByString() ValidatorSortByFunc {
	return func(v1 Validator, v2 Validator) bool {
		return strings.Compare(v1.String(), v2.String()) < 0
	}
}

func ValidatorSortByByte() ValidatorSortByFunc {
	return func(v1 Validator, v2 Validator) bool {
		return bytes.Compare(v1.Address().Bytes(), v2.Address().Bytes()) < 0
	}
}

func (by ValidatorSortByFunc) Sort(validators []Validator) {
	v := &validatorSorter{
		validators: validators,
		by:         by,
	}
	sort.Sort(v)
}

// ----------------------------------------------------------------------------

type ValidatorSet interface {
	// Calculate the proposer
	CalcProposer(lastProposer libcommon.Address, round uint64)
	// Return the validator size
	Size() int
	// Return the validator array
	List() []Validator
	// Get validator by index
	GetByIndex(i uint64) Validator
	// Get validator by given address
	GetByAddress(addr libcommon.Address) (int, Validator)
	// Get current proposer
	GetProposer() Validator
	// Check whether the validator with given address is a proposer
	IsProposer(address libcommon.Address) bool
	// Add validator
	AddValidator(address libcommon.Address) bool
	// Remove validator
	RemoveValidator(address libcommon.Address) bool
	// Copy validator set
	Copy() ValidatorSet
	// Get the maximum number of faulty nodes
	F() int
	// Get proposer policy
	Policy() ProposerPolicy

	// SortValidators sorts the validators based on the configured By function
	SortValidators()
}

// ----------------------------------------------------------------------------

type ProposalSelector func(ValidatorSet, libcommon.Address, uint64) Validator
