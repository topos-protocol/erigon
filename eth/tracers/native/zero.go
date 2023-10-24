package native

import (
	"encoding/json"
	"errors"
	"fmt"

	"sync/atomic"

	"github.com/holiman/uint256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/crypto"
	"github.com/ledgerwatch/erigon/eth/tracers"
	"github.com/ledgerwatch/log/v3"
)

//go:generate go run github.com/fjl/gencodec -type account -field-override accountMarshaling -out gen_account_json.go

func init() {
	register("zeroTracer", newZeroTracer)
}

type Account struct {
	Balance      *uint256.Int                    `json:"balance,omitempty"`
	Nonce        uint64                          `json:"nonce,omitempty"`
	ReadStorage  map[libcommon.Hash]*uint256.Int `json:"storage_read,omitempty"`
	WriteStorage map[libcommon.Hash]*uint256.Int `json:"storage_write,omitempty"`
	CodeUsage    string                          `json:"code_usage,omitempty"`
}

type TX struct {
	ByteCode string                         `json:"byte_code,omitempty"` // TX CallData
	GasUsed  uint64                         `json:"gas_used,omitempty"`
	Trace    map[libcommon.Address]*Account `json:"trace,omitempty"`
}

type zeroTracer struct {
	noopTracer // stub struct to mock not used interface methods
	env        vm.VMInterface
	tx         TX
	interrupt  atomic.Bool // Atomic flag to signal execution interruption
	reason     error       // Textual reason for the interruption
}

func newZeroTracer(ctx *tracers.Context, cfg json.RawMessage) (tracers.Tracer, error) {
	return &zeroTracer{
		tx: TX{
			Trace: make(map[libcommon.Address]*Account),
		},
	}, nil
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *zeroTracer) CaptureStart(env vm.VMInterface, from libcommon.Address, to libcommon.Address, precompile, create bool, input []byte, gas uint64, value *uint256.Int, code []byte) {
	t.env = env
	t.tx.ByteCode = common.Bytes2Hex(input)

	t.addAccountToTrace(from, false)
	t.addAccountToTrace(to, false)
	t.addAccountToTrace(env.Context().Coinbase, false)
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *zeroTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	if err != nil {
		return
	}

	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
	}

	stack := scope.Stack
	stackData := stack.Data
	stackLen := len(stackData)
	caller := scope.Contract.Address()

	switch {
	case stackLen >= 1 && op == vm.SLOAD:
		slot := libcommon.Hash(stackData[stackLen-1].Bytes32())
		t.addSLOADToAccount(caller, slot)
	case stackLen >= 1 && op == vm.SSTORE:
		slot := libcommon.Hash(stackData[stackLen-1].Bytes32())
		t.addSSTOREToAccount(caller, slot)
	case stackLen >= 1 && (op == vm.EXTCODECOPY || op == vm.EXTCODEHASH || op == vm.EXTCODESIZE || op == vm.BALANCE || op == vm.SELFDESTRUCT):
		addr := libcommon.Address(stackData[stackLen-1].Bytes20())
		t.addAccountToTrace(addr, false)
	case stackLen >= 5 && (op == vm.DELEGATECALL || op == vm.CALL || op == vm.STATICCALL || op == vm.CALLCODE):
		addr := libcommon.Address(stackData[stackLen-2].Bytes20())
		t.addAccountToTrace(addr, false)
	case op == vm.CREATE:
		nonce := t.env.IntraBlockState().GetNonce(caller)
		addr := crypto.CreateAddress(caller, nonce)
		t.addAccountToTrace(addr, true)
	case stackLen >= 4 && op == vm.CREATE2:
		offset := stackData[stackLen-2]
		size := stackData[stackLen-3]
		init, err := GetMemoryCopyPadded(scope.Memory, int64(offset.Uint64()), int64(size.Uint64()))
		if err != nil {
			log.Warn("failed to copy CREATE2 input", "err", err, "tracer", "prestateTracer", "offset", offset, "size", size)
			return
		}
		inithash := crypto.Keccak256(init)
		salt := stackData[stackLen-4]
		addr := crypto.CreateAddress2(caller, salt.Bytes32(), inithash)
		t.addAccountToTrace(addr, true)
	}
}

const (
	memoryPadLimit = 1024 * 1024
)

// GetMemoryCopyPadded returns offset + size as a new slice.
// It zero-pads the slice if it extends beyond memory bounds.
func GetMemoryCopyPadded(m *vm.Memory, offset, size int64) ([]byte, error) {
	if offset < 0 || size < 0 {
		return nil, errors.New("offset or size must not be negative")
	}
	if int(offset+size) < m.Len() { // slice fully inside memory
		return m.GetCopy(offset, size), nil
	}
	paddingNeeded := int(offset+size) - m.Len()
	if paddingNeeded > memoryPadLimit {
		return nil, fmt.Errorf("reached limit for padding memory slice: %d", paddingNeeded)
	}
	cpy := make([]byte, size)
	if overlap := int64(m.Len()) - offset; overlap > 0 {
		copy(cpy, m.GetPtr(offset, overlap))
	}
	return cpy, nil
}

func (t *zeroTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	t.tx.GasUsed = gasUsed
}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *zeroTracer) GetResult() (json.RawMessage, error) {
	var res []byte
	var err error
	res, err = json.Marshal(t.tx)

	if err != nil {
		return nil, err
	}

	return json.RawMessage(res), t.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *zeroTracer) Stop(err error) {
	t.reason = err
	t.interrupt.Store(true)
}

func (t *zeroTracer) addAccountToTrace(addr libcommon.Address, created bool) {
	if _, ok := t.tx.Trace[addr]; ok {
		return
	}

	var code string
	if created {
		code = common.Bytes2Hex(t.env.IntraBlockState().GetCode(addr))
	} else {
		code = libcommon.Hash.String(t.env.IntraBlockState().GetCodeHash(addr))
	}

	t.tx.Trace[addr] = &Account{
		Balance:      t.env.IntraBlockState().GetBalance(addr),
		Nonce:        t.env.IntraBlockState().GetNonce(addr),
		CodeUsage:    code,
		WriteStorage: make(map[libcommon.Hash]*uint256.Int),
		ReadStorage:  make(map[libcommon.Hash]*uint256.Int),
	}
}

func (t *zeroTracer) addSLOADToAccount(addr libcommon.Address, key libcommon.Hash) {
	var value uint256.Int
	t.env.IntraBlockState().GetState(addr, &key, &value)
	t.tx.Trace[addr].ReadStorage[key] = &value
}

func (t *zeroTracer) addSSTOREToAccount(addr libcommon.Address, key libcommon.Hash) {
	var value uint256.Int
	t.env.IntraBlockState().GetState(addr, &key, &value)
	t.tx.Trace[addr].WriteStorage[key] = &value
}
