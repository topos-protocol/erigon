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
	"bytes"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/length"
	"github.com/ledgerwatch/erigon/consensus/istanbul"
)

func New(addr libcommon.Address) istanbul.Validator {
	return &defaultValidator{
		address: addr,
	}
}

func NewSet(addrs []libcommon.Address, policy *istanbul.ProposerPolicy) istanbul.ValidatorSet {
	return newDefaultSet(addrs, policy)
}

func ExtractValidators(extraData []byte) []libcommon.Address {
	// get the validator addresses
	addrs := make([]libcommon.Address, (len(extraData) / length.Addr))
	for i := 0; i < len(addrs); i++ {
		copy(addrs[i][:], extraData[i*length.Addr:])
	}

	return addrs
}

// Check whether the extraData is presented in prescribed form
func ValidExtraData(extraData []byte) bool {
	return len(extraData)%length.Addr == 0
}

func SortedAddresses(validators []istanbul.Validator) []libcommon.Address {
	addrs := make([]libcommon.Address, len(validators))
	for i, validator := range validators {
		addrs[i] = validator.Address()
	}

	for i := 0; i < len(addrs); i++ {
		for j := i + 1; j < len(addrs); j++ {
			if bytes.Compare(addrs[i][:], addrs[j][:]) > 0 {
				addrs[i], addrs[j] = addrs[j], addrs[i]
			}
		}
	}

	return addrs
}
