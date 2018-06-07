/*
	Copyright 2017-2018 OneLedger

	An incoming transaction, send, swap, ready, verification, etc.
*/
package action

import (
	"github.com/Oneledger/protocol/node/comm"
	"github.com/Oneledger/protocol/node/data"
	"github.com/Oneledger/protocol/node/err"
	"github.com/Oneledger/protocol/node/log"
	"github.com/Oneledger/protocol/node/persist"
)

// Synchronize a swap between two users
type Send struct {
	Base

	Inputs  []SendInput  `json:"inputs"`
	Outputs []SendOutput `json:"outputs"`

	Gas data.Coin `json:"gas"`
	Fee data.Coin `json:"fee"`
}

func (transaction *Send) Validate() err.Code {
	log.Debug("Validating Send Transaction")

	// TODO: Make sure all of the parameters are there
	// TODO: Check all signatures and keys
	// TODO: Vet that the sender has the values
	return err.SUCCESS
}

func (transaction *Send) ProcessCheck(app interface{}) err.Code {
	log.Debug("Processing Send Transaction for CheckTx")

	// TODO: Validate the transaction against the UTXO database, check tree

	return err.SUCCESS
}

func (transaction *Send) ProcessDeliver(app interface{}) err.Code {
	log.Debug("Processing Send Transaction for DeliverTx")

	chain := app.(persist.Access).GetUtxo().(*data.ChainState)

	// TODO: Revalidate the transaction
	// TODO: Need to rollback if any errors occur

	// Update the database to the final set of entries
	for _, entry := range transaction.Outputs {
		value, _ := comm.Serialize(entry.Coin)
		chain.Delivered.Set(entry.Address, value)
	}

	return err.SUCCESS
}

// Given a transaction, expand it into a list of Commands to execute against various chains.
func (transaction *Send) Expand(app interface{}) Commands {
	// TODO: Table-driven mechanics, probably elsewhere
	return []Command{}
}
