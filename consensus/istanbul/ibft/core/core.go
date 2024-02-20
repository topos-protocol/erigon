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

package core

import (
	"bytes"
	"math"
	"math/big"
	"sync"
	"time"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	metrics "github.com/ledgerwatch/erigon-lib/metrics"
	"github.com/ledgerwatch/erigon/consensus"
	"github.com/ledgerwatch/erigon/consensus/istanbul"
	ibfttypes "github.com/ledgerwatch/erigon/consensus/istanbul/ibft/types"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/event"
	"github.com/ledgerwatch/log/v3"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

var (
	roundMeter     = metrics.NewCounter("consensus/istanbul/core/round")
	sequenceMeter  = metrics.NewCounter("consensus/istanbul/core/sequence")
	consensusTimer = metrics.NewHistTimer("consensus/istanbul/core/consensus")
)

// New creates an Istanbul consensus core
func New(backend istanbul.Backend, config *istanbul.Config) *core {
	c := &core{
		config:             config,
		address:            backend.Address(),
		state:              ibfttypes.StateAcceptRequest,
		handlerWg:          new(sync.WaitGroup),
		logger:             log.New("address", backend.Address()),
		backend:            backend,
		backlogs:           make(map[libcommon.Address]*prque.Prque),
		backlogsMu:         new(sync.Mutex),
		pendingRequests:    prque.New(),
		pendingRequestsMu:  new(sync.Mutex),
		consensusTimestamp: time.Time{},
	}

	c.validateFn = c.checkValidatorSignature
	return c
}

// ----------------------------------------------------------------------------

type core struct {
	config  *istanbul.Config
	address libcommon.Address
	state   ibfttypes.State
	logger  log.Logger

	backend               istanbul.Backend
	events                *event.TypeMuxSubscription
	finalCommittedSub     *event.TypeMuxSubscription
	timeoutSub            *event.TypeMuxSubscription
	futurePreprepareTimer *time.Timer

	valSet                consensus.ValidatorSet
	waitingForRoundChange bool
	validateFn            func([]byte, []byte) (libcommon.Address, error)

	backlogs   map[libcommon.Address]*prque.Prque
	backlogsMu *sync.Mutex

	current   *roundState
	handlerWg *sync.WaitGroup

	roundChangeSet   *roundChangeSet
	roundChangeTimer *time.Timer

	pendingRequests   *prque.Prque
	pendingRequestsMu *sync.Mutex

	consensusTimestamp time.Time
}

func (c *core) finalizeMessage(msg *ibfttypes.Message) ([]byte, error) {
	var err error
	// Add sender address
	msg.Address = c.Address()

	// Assign the CommittedSeal if it's a COMMIT message and proposal is not nil
	if msg.Code == ibfttypes.MsgCommit && c.current.Proposal() != nil {
		msg.CommittedSeal = []byte{}
		seal := PrepareCommittedSeal(c.current.Proposal().Hash())
		// Add proof of consensus
		msg.CommittedSeal, err = c.backend.Sign(seal)
		if err != nil {
			return nil, err
		}
	}

	// Sign message
	data, err := msg.PayloadNoSig()
	if err != nil {
		return nil, err
	}
	msg.Signature, err = c.backend.Sign(data)
	if err != nil {
		return nil, err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (c *core) broadcast(msg *ibfttypes.Message) {
	logger := c.logger.New("state", c.state)

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}

	// Broadcast payload
	if err = c.backend.Broadcast(c.valSet, msg.Code, payload); err != nil {
		logger.Error("Failed to broadcast message", "msg", msg, "err", err)
		return
	}
}

func (c *core) currentView() *istanbul.View {
	return &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Set(c.current.Round()),
	}
}

func (c *core) IsProposer() bool {
	v := c.valSet
	if v == nil {
		return false
	}
	return v.IsProposer(c.backend.Address())
}

func (c *core) IsCurrentProposal(blockHash libcommon.Hash) bool {
	return c.current != nil && c.current.pendingRequest != nil && c.current.pendingRequest.Proposal.Hash() == blockHash
}

func (c *core) commit() {
	c.setState(ibfttypes.StateCommitted)

	proposal := c.current.Proposal()
	if proposal != nil {
		committedSeals := make([][]byte, c.current.Commits.Size())
		for i, v := range c.current.Commits.Values() {
			committedSeals[i] = make([]byte, types.IstanbulExtraSeal)
			copy(committedSeals[i][:], v.CommittedSeal[:])
		}

		if err := c.backend.Commit(proposal, committedSeals, big.NewInt(-1)); err != nil {
			c.current.UnlockHash() //Unlock block when insertion fails
			c.sendNextRoundChange()
			return
		}
	}
}

// startNewRound starts a new round. if round equals to 0, it means to starts a new sequence
func (c *core) startNewRound(round *big.Int) {
	var logger log.Logger
	if c.current == nil {
		logger = c.logger.New("old_round", -1, "old_seq", 0)
	} else {
		logger = c.logger.New("old_round", c.current.Round(), "old_seq", c.current.Sequence())
	}

	logger.Trace("Start new ibft round")

	roundChange := false
	// Try to get last proposal
	lastProposal, lastProposer := c.backend.LastProposal()
	if c.current == nil {
		logger.Trace("Start to the initial round")
	} else if lastProposal.Number().Cmp(c.current.Sequence()) >= 0 {
		diff := new(big.Int).Sub(lastProposal.Number(), c.current.Sequence())
		// sequenceMeter.Mark(new(big.Int).Add(diff, libcommon.Big1).Int64())
		sequenceMeter.AddUint64(new(big.Int).Add(diff, libcommon.Big1).Uint64())

		if !c.consensusTimestamp.IsZero() {
			// consensusTimer.UpdateSince(c.consensusTimestamp)
			consensusTimer.PutSince()
			c.consensusTimestamp = time.Time{}
		}
		logger.Trace("Catch up latest proposal", "number", lastProposal.Number().Uint64(), "hash", lastProposal.Hash())
	} else if lastProposal.Number().Cmp(big.NewInt(c.current.Sequence().Int64()-1)) == 0 {
		if round.Cmp(libcommon.Big0) == 0 {
			// same seq and round, don't need to start new round
			return
		} else if round.Cmp(c.current.Round()) < 0 {
			logger.Warn("New round should not be smaller than current round", "seq", lastProposal.Number().Int64(), "new_round", round, "old_round", c.current.Round())
			return
		}
		roundChange = true
	} else {
		logger.Warn("New sequence should be larger than current sequence", "new_seq", lastProposal.Number().Int64())
		return
	}

	var newView *istanbul.View
	if roundChange {
		newView = &istanbul.View{
			Sequence: new(big.Int).Set(c.current.Sequence()),
			Round:    new(big.Int).Set(round),
		}
	} else {
		newView = &istanbul.View{
			Sequence: new(big.Int).Add(lastProposal.Number(), libcommon.Big1),
			Round:    new(big.Int),
		}
		c.valSet = c.backend.Validators(lastProposal)
	}

	// If new round is 0, then check if qbftConsensus needs to be enabled
	if round.Uint64() == 0 && c.backend.IsQBFTConsensusAt(newView.Sequence) {
		logger.Trace("Starting qbft consensus as qbftBlock has passed")
		if err := c.backend.StartQBFTConsensus(); err != nil {
			// If err is returned, then QBFT consensus is started for the next block
			logger.Error("Unable to start QBFT Consensus, retrying for the next block", "error", err)
		}
		return
	}

	// Update logger
	logger = logger.New("old_proposer", c.valSet.GetProposer())
	// Clear invalid ROUND CHANGE messages
	c.roundChangeSet = newRoundChangeSet(c.valSet)
	// New snapshot for new round
	c.updateRoundState(newView, c.valSet, roundChange)
	// Calculate new proposer
	c.valSet.CalcProposer(lastProposer, newView.Round.Uint64())
	c.waitingForRoundChange = false
	c.setState(ibfttypes.StateAcceptRequest)
	if roundChange && c.IsProposer() && c.current != nil {
		// If it is locked, propose the old proposal
		// If we have pending request, propose pending request
		if c.current.IsHashLocked() {
			r := &istanbul.Request{
				Proposal: c.current.Proposal(), //c.current.Proposal would be the locked proposal by previous proposer, see updateRoundState
			}
			c.sendPreprepare(r)
		} else if c.current.pendingRequest != nil {
			c.sendPreprepare(c.current.pendingRequest)
		}
	}
	c.newRoundChangeTimer()

	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence, "new_proposer", c.valSet.GetProposer(), "valSet", c.valSet.List(), "size", c.valSet.Size(), "IsProposer", c.IsProposer())
}

func (c *core) catchUpRound(view *istanbul.View) {
	logger := c.logger.New("old_round", c.current.Round(), "old_seq", c.current.Sequence(), "old_proposer", c.valSet.GetProposer())

	if view.Round.Cmp(c.current.Round()) > 0 {
		// roundMeter.Mark(new(big.Int).Sub(view.Round, c.current.Round()).Int64())
		roundMeter.AddUint64(new(big.Int).Sub(view.Round, c.current.Round()).Uint64())
	}
	c.waitingForRoundChange = true

	// Need to keep block locked for round catching up
	c.updateRoundState(view, c.valSet, true)
	c.roundChangeSet.Clear(view.Round)
	c.newRoundChangeTimer()

	logger.Trace("Catch up round", "new_round", view.Round, "new_seq", view.Sequence, "new_proposer", c.valSet)
}

// updateRoundState updates round state by checking if locking block is necessary
func (c *core) updateRoundState(view *istanbul.View, validatorSet consensus.ValidatorSet, roundChange bool) {
	// Lock only if both roundChange is true and it is locked
	if roundChange && c.current != nil {
		if c.current.IsHashLocked() {
			c.current = newRoundState(view, validatorSet, c.current.GetLockedHash(), c.current.Preprepare, c.current.pendingRequest, c.backend.HasBadProposal)
		} else {
			c.current = newRoundState(view, validatorSet, libcommon.Hash{}, nil, c.current.pendingRequest, c.backend.HasBadProposal)
		}
	} else {
		c.current = newRoundState(view, validatorSet, libcommon.Hash{}, nil, nil, c.backend.HasBadProposal)
	}
}

func (c *core) setState(state ibfttypes.State) {
	if c.state != state {
		c.state = state
	}
	if state == ibfttypes.StateAcceptRequest {
		c.processPendingRequests()
	}
	c.processBacklog()
}

func (c *core) Address() libcommon.Address {
	return c.address
}

func (c *core) stopFuturePreprepareTimer() {
	if c.futurePreprepareTimer != nil {
		c.futurePreprepareTimer.Stop()
	}
}

func (c *core) stopTimer() {
	c.stopFuturePreprepareTimer()
	if c.roundChangeTimer != nil {
		c.roundChangeTimer.Stop()
	}
}

func (c *core) newRoundChangeTimer() {
	c.stopTimer()

	// set timeout based on the round number
	timeout := time.Duration(c.config.GetConfig(c.current.Sequence()).RequestTimeout) * time.Millisecond
	round := c.current.Round().Uint64()
	if round > 0 {
		timeout += time.Duration(math.Pow(2, float64(round))) * time.Second
	}
	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendEvent(timeoutEvent{})
	})
}

func (c *core) checkValidatorSignature(data []byte, sig []byte) (libcommon.Address, error) {
	return istanbul.CheckValidatorSignature(c.valSet, data, sig)
}

func (c *core) QuorumSize() int {
	if c.config.Get2FPlus1Enabled(c.current.sequence) || c.config.Ceil2Nby3Block == nil || (c.current != nil && c.current.sequence.Cmp(c.config.Ceil2Nby3Block) < 0) {
		c.logger.Trace("Confirmation Formula used 2F+ 1")
		return (2 * c.valSet.F()) + 1
	}
	c.logger.Trace("Confirmation Formula used ceil(2N/3) >>>>>>>>>>>>")
	return int(math.Ceil(float64(2*c.valSet.Size()) / 3))
}

// PrepareCommittedSeal returns a committed seal for the given hash
func PrepareCommittedSeal(hash libcommon.Hash) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write([]byte{byte(ibfttypes.MsgCommit)})
	return buf.Bytes()
}
