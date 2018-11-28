package runner

import (
	"github.com/Oneledger/protocol/node/serial"
	"github.com/robertkrimen/otto"
)

type Runner struct {
	vm *otto.Otto
}

// All of the input necessary to perform a computation on a transaction
type OLVMRequest struct {
	// TODO: Original Transaction
	// TODO: Last execution context
	// TODO: Scripts (if we can follow the includes and get all of them)
	// TODO: Data Handle (some way to call out for large data requests)

	From       string
	Address    string
	CallString string
	Value      int
	SourceCode string
}

// All of the output received from the computation
type OLVMResult struct {
	// TODO: Any subseqeunce transaction that needs to be broadcasted
	// TODO: Last execution context

	Out     string
	Ret     string // TODO: Should be a real name
	Elapsed string
}

func init() {
	serial.Register(OLVMRequest{})
	serial.Register(OLVMResult{})

	// Doesn't work in serial?
	//var prototype time.Time
	//serial.Register(prototype)
	//var prototype2 time.Duration
	//serial.Register(prototype2)
}
