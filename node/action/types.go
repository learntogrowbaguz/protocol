/*
	Copyright 2017-2018 OneLedger

	Declare basic types used by the Application

	If a type requires functions or a few types are intertwinded, then should be in their own file.
*/
package action

import (
	"bytes"

	"strconv"

	"github.com/Oneledger/protocol/node/data"
	"github.com/Oneledger/protocol/node/id"
	"github.com/Oneledger/protocol/node/log"
	"github.com/Oneledger/protocol/node/serial"
)

// inputs into a send transaction (similar to Bitcoin)
type SendInput struct {
	AccountKey id.AccountKey `json:"account_key"`
	PubKey     PublicKey     `json:"pub_key"`
	Signature  id.Signature  `json:"signature"`

	Amount data.Coin `json:"coin"`

	// TODO: Is sequence needed per input?
	Sequence int `json:"sequence"`
}

func init() {
	serial.Register(SendInput{})
}

func NewSendInput(accountKey id.AccountKey, amount data.Coin) SendInput {
	if bytes.Equal(accountKey, []byte("")) {
		log.Fatal("Missing AccountKey")
	}

	return SendInput{
		AccountKey: accountKey,
		Amount:     amount,
	}
}

// outputs for a send transaction (similar to Bitcoin)
type SendOutput struct {
	AccountKey id.AccountKey `json:"account_key"`
	Amount     data.Coin     `json:"coin"`
}

func init() {
	serial.Register(SendOutput{})
}

func NewSendOutput(accountKey id.AccountKey, amount data.Coin) SendOutput {
	if bytes.Equal(accountKey, []byte("")) {
		log.Fatal("Missing AccountKey")
	}

	return SendOutput{
		AccountKey: accountKey,
		Amount:     amount,
	}
}

func CheckBalance(app interface{}, accountKey id.AccountKey, amount data.Coin) bool {
	utxo := GetUtxo(app)

	version := utxo.Delivered.Version64()
	_, value := utxo.Delivered.GetVersioned(accountKey, version)
	if value == nil {
		log.Debug("Key not in database, setting to zero", "key", accountKey)
		return true
	}

	var bal data.Balance
	buffer, err := serial.Deserialize(value, bal, serial.CLIENT)
	if err != nil || buffer == nil {
		log.Error("Failed to Deserialize", "key", accountKey)
		return false
	}

	balance := buffer.(data.Balance)
	if !balance.Amount.Equals(amount) {
		log.Warn("Mismatch", "key", accountKey, "amount", amount, "balance", balance)
		//return false
	}
	return true
}

func GetHeight(app interface{}) int64 {
	utxo := GetUtxo(app)

	height := int64(utxo.Height)
	return height
}

func CheckAmounts(app interface{}, inputs []SendInput, outputs []SendOutput) bool {
	total := data.NewCoin(0, "OLT")
	for _, input := range inputs {
		if input.Amount.LessThan(0) {
			log.Debug("Less Than 0", "input", input)
			return false
		}

		if !input.Amount.IsCurrency("OLT") {
			log.Debug("Send on Currency isn't implement yet")
			return false
		}

		if bytes.Compare(input.AccountKey, []byte("")) == 0 {
			log.Debug("Key is Empty", "input", input)
			return false
		}
		if !CheckBalance(app, input.AccountKey, input.Amount) {
			log.Debug("Balance is missing", "input", input)

			// TODO: Temporarily disabled
			//return false
		}
		total.Plus(input.Amount)
	}
	for _, output := range outputs {

		if output.Amount.LessThan(0) {
			log.Debug("Less Than 0", "output", output)
			return false
		}

		if !output.Amount.IsCurrency("OLT") {
			log.Debug("Send on Currency isn't implement yet")
			return false
		}

		if bytes.Compare(output.AccountKey, []byte("")) == 0 {
			log.Debug("Key is Empty", "output", output)
			return false
		}
		total.Minus(output.Amount)
	}
	if !total.Equals(data.NewCoin(0, "OLT")) {
		log.Debug("Doesn't add up", "inputs", inputs, "outputs", outputs)
		return false
	}
	return true
}

type Event struct {
	Type  Type          `json:"type"`
	Key   id.AccountKey `json:"key"`
	Nonce int64         `json:"result"`
}

func (e Event) ToKey() []byte {
	buffer, err := serial.Serialize(e, serial.CLIENT)
	if err != nil {
		log.Error("Failed to Serialize event key")
	}
	return buffer
}

func SaveEvent(app interface{}, eventKey Event, status bool) {
	events := GetEvent(app)

	log.Debug("Save Event", "key", eventKey)

	events.Store(eventKey.ToKey(), []byte(strconv.FormatBool(status)))
	events.Commit()
}

func FindEvent(app interface{}, eventKey Event) bool {
	log.Debug("Load Event", "key", eventKey)
	events := GetEvent(app)
	result := events.Load(eventKey.ToKey())
	if result == nil {
		return false
	}

	r, err := strconv.ParseBool(string(result))
	if err != nil {
		return false
	}

	return r
}

func SaveContract(app interface{}, contractKey []byte, nonce int64, contract []byte) {
	contracts := GetContract(app)
	n := strconv.AppendInt(contractKey, nonce, 10)
	log.Debug("Save contract", "key", contractKey, "afterNonce", n)
	contracts.Store(n, contract)
	contracts.Commit()
}

func FindContract(app interface{}, contractKey []byte, nonce int64) []byte {
	log.Debug("Load Contract", "key", contractKey)
	contracts := GetContract(app)
	n := strconv.AppendInt(contractKey, nonce, 10)
	result := contracts.Load(n)
	if result == nil {
		return nil
	}
	return result
}
