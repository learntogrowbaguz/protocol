/*
	Copyright 2017 - 2018 OneLedger

	Structures and functions for getting command line arguments, and functions
	to convert these into specific requests.
*/
package shared

import (
	"os"

	"github.com/Oneledger/protocol/node/action"
	"github.com/Oneledger/protocol/node/app"
	"github.com/Oneledger/protocol/node/convert"
	"github.com/Oneledger/protocol/node/data"
	"github.com/Oneledger/protocol/node/log"
)

// Prepare a transaction to be issued.
func SignAndPack(transaction action.Transaction) []byte {
	return action.SignAndPack(transaction)
}

// Registration
type AccountArguments struct {
	Account     string
	Chain       string
	PublicKey   string
	PrivateKey  string
	NodeAccount bool
}

func UpdateAccountRequest(args *AccountArguments) interface{} {
	return &app.SDKSet{
		Path: "/account",
		Arguments: map[string]string{
			"Account":     args.Account,
			"Chain":       args.Chain,
			"PublicKey":   args.PublicKey,
			"PrivateKey":  args.PrivateKey,
			"NodeAccount": "true",
		},
	}
}

// Registration
type RegisterArguments struct {
	Identity string
	Account  string
	NodeName string
}

// Create a request to register a new identity with the chain
func RegisterIdentityRequest(args *RegisterArguments) interface{} {
	//signers := GetSigners()

	// TODO: Need to check errors here
	//accountKey := GetAccountKey(args.Account)

	// TODO: Need to have access to this data?
	//app.LoadPrivValidatorFile()

	return &app.SDKSet{
		Path: "/register",
		Arguments: map[string]string{
			"Identity": args.Identity,
			"Account":  args.Account,
			"NodeName": args.NodeName,
		},
	}
}

type BalanceArguments struct {
}

func CreateBalanceRequest(args *BalanceArguments) []byte {
	return []byte(nil)
}

type ApplyValidatorArguments struct {
	Id           string
	Amount       string
}

// CreateRequest builds and signs the transaction based on the arguments
func CreateApplyValidatorRequest(args *ApplyValidatorArguments) []byte {
	conv := convert.NewConvert()

	party := GetAccountKey(args.Id)
	if party == nil {
		log.Fatal("System doesn't recognize the parties", "args", args, "party", party)
	}

	if args.Amount == "" {
		log.Error("Missing an amount argument")
		return nil
	}

	balance := GetBalance(party).GetAmountByName("VT")

	log.Dump("ValidatorBalance", "balance", balance)

	if &balance == nil {
		log.Error("Missing Balance", "balance", balance)
		return nil
	}

	amount := conv.GetCoin(args.Amount, "VT")

	// Create base transaction
	// TODO Need to populate all fields correctly
	validator := &action.ApplyValidator{
		Base: action.Base{
			Type:     action.APPLY_VALIDATOR,
			ChainId:  app.ChainId,
			Owner:    party,
			Signers:  action.GetSigners(party),
			Sequence: GetSequenceNumber(party),
		},

		AccountKey:         party,
		TendermintAddress: "SomeAddress",
		TendermintPubKey:  "SomePubKey",
		Stake:             amount,
	}

	return SignAndPack(action.Transaction(validator))
}

type SendArguments struct {
	Party        string
	CounterParty string
	Currency     string
	Amount       string
	Gas          string
	Fee          string
}

// CreateRequest builds and signs the transaction based on the arguments
func CreateSendRequest(args *SendArguments) []byte {
	conv := convert.NewConvert()

	if args.Party == "" {
		log.Error("Missing Party argument")
		return nil
	}

	if args.CounterParty == "" {
		log.Error("Missing CounterParty argument")
		return nil
	}

	// TODO: Can't convert identities to accounts, this way!
	party := GetAccountKey(args.Party)
	counterParty := GetAccountKey(args.CounterParty)
	payment := GetAccountKey("Payment")
	if party == nil || counterParty == nil {
		log.Fatal("System doesn't reconize the parties", "args", args,
			"party", party, "counterParty", counterParty)
		//return nil
	}

	if args.Currency == "" || args.Amount == "" {
		log.Error("Missing an amount argument")
		return nil
	}

	amount := conv.GetCoin(args.Amount, args.Currency)

	// Build up the Inputs
	partyBalance := GetBalance(party).GetAmountByName(args.Currency)
	counterPartyBalance := GetBalance(counterParty).GetAmountByName(args.Currency)
	paymentBalance := GetBalance(payment).GetAmountByName(args.Currency)

	//log.Dump("Balances", partyBalance, counterPartyBalance)

	if &partyBalance == nil || &counterPartyBalance == nil {
		log.Error("Missing Balance", "party", partyBalance, "counterParty", counterPartyBalance)
		return nil
	}

	fee := conv.GetCoin(args.Fee, args.Currency)
	gas := conv.GetCoin(args.Gas, args.Currency)

	inputs := make([]action.SendInput, 0)
	inputs = append(inputs,
		action.NewSendInput(party, partyBalance),
		action.NewSendInput(counterParty, counterPartyBalance),
		action.NewSendInput(payment, paymentBalance))

	// Build up the outputs
	outputs := make([]action.SendOutput, 0)
	outputs = append(outputs,
		action.NewSendOutput(party, partyBalance.Minus(amount).Minus(fee)),
		action.NewSendOutput(counterParty, counterPartyBalance.Plus(amount)),
		action.NewSendOutput(payment, paymentBalance.Plus(fee)))

	if conv.HasErrors() {
		Console.Error(conv.GetErrors())
		os.Exit(-1)
	}

	sequence := GetSequenceNumber(party)

	// Create base transaction
	send := &action.Send{
		Base: action.Base{
			Type:     action.SEND,
			ChainId:  app.ChainId,
			Owner:    party,
			Signers:  action.GetSigners(party),
			Sequence: sequence,
		},
		Inputs:  inputs,
		Outputs: outputs,
		Fee:     fee,
		Gas:     gas,
	}
	return SignAndPack(action.Transaction(send))
}

// CreateRequest builds and signs the transaction based on the arguments
func CreateMintRequest(args *SendArguments) []byte {
	conv := convert.NewConvert()

	if args.Party == "" {
		log.Warn("Missing Party arguments", "args", args)
		return nil
	}

	zero := GetAccountKey("Zero")
	party := GetAccountKey(args.Party)

	if party == nil || zero == nil {
		log.Warn("Missing Party information", "args", args, "party", party, "zero", zero)
		return nil
	}

	amount := conv.GetCoin(args.Amount, args.Currency)

	// Build up the Inputs
	log.Debug("Getting TestMint Account Balances")
	partyBalance := GetBalance(party).GetAmountByName(args.Currency)
	zeroBalance := GetBalance(zero).GetAmountByName(args.Currency)

	if &zeroBalance == nil || &partyBalance == nil {
		log.Warn("Missing Balances", "party", party, "zero", zero)
		return nil
	}

	if zeroBalance.LessThanEqual(0) {
		log.Warn("No more money left...")
		return nil
	}

	inputs := make([]action.SendInput, 0)
	inputs = append(inputs,
		action.NewSendInput(zero, zeroBalance),
		action.NewSendInput(party, partyBalance))

	// Build up the outputs
	outputs := make([]action.SendOutput, 0)
	outputs = append(outputs,
		action.NewSendOutput(zero, zeroBalance.Minus(amount)),
		action.NewSendOutput(party, partyBalance.Plus(amount)))

	gas := conv.GetCoin(args.Gas, args.Currency)
	fee := conv.GetCoin(args.Fee, args.Currency)

	if conv.HasErrors() {
		Console.Error(conv.GetErrors())
		os.Exit(-1)
	}

	sequence := GetSequenceNumber(party)

	// Create base transaction
	send := &action.Send{
		Base: action.Base{
			Type:     action.SEND,
			ChainId:  app.ChainId,
			Signers:  action.GetSigners(zero),
			Owner:    zero,
			Sequence: sequence,
		},
		Inputs:  inputs,
		Outputs: outputs,
		Fee:     fee,
		Gas:     gas,
	}
	return SignAndPack(action.Transaction(send))
}

// Arguments to the command
type SwapArguments struct {
	Party        string
	CounterParty string
	Amount       string
	Currency     string
	Fee          string
	Gas          string // TODO: Not sure this is necessary, unless the chain is like Ethereum
	Exchange     string
	Excurrency   string
	Nonce        int64
}

// Create a swap request
func CreateSwapRequest(args *SwapArguments) []byte {

	conv := convert.NewConvert()

	partyKey := GetAccountKey(args.Party)
	counterPartyKey := GetAccountKey(args.CounterParty)

	fee := conv.GetCoin(args.Fee, "OLT")
	gas := conv.GetCoin(args.Gas, "OLT")

	amount := conv.GetCoin(args.Amount, args.Currency)
	exchange := conv.GetCoin(args.Exchange, args.Excurrency)

	if conv.HasErrors() {
		Console.Error(conv.GetErrors())
		os.Exit(-1)
	}
	account := make(map[data.ChainType][]byte)
	counterAccount := make(map[data.ChainType][]byte)

	account[conv.GetChainFromCurrency(args.Currency)] = GetCurrencyAddress(conv.GetCurrency(args.Currency), args.Party)
	account[conv.GetChainFromCurrency(args.Excurrency)] = GetCurrencyAddress(conv.GetCurrency(args.Excurrency), args.Party)
	//log.Debug("accounts for swap", "accountbtc", account[data.BITCOIN], "accounteth", common.BytesToAddress([]byte(account[data.ETHEREUM])), "accountolt", account[data.ONELEDGER])

	party := action.Party{Key: partyKey, Accounts: account}
	counterParty := action.Party{Key: counterPartyKey, Accounts: counterAccount}

	swapInit := action.SwapInit{
		Party:        party,
		CounterParty: counterParty,
		Fee:          fee,
		Gas:          gas,
		Amount:       amount,
		Exchange:     exchange,
		Nonce:        args.Nonce,
	}

	sequence := GetSequenceNumber(partyKey)

	swap := &action.Swap{
		Base: action.Base{
			Type:     action.SWAP,
			ChainId:  app.ChainId,
			Signers:  action.GetSigners(partyKey),
			Owner:    partyKey,
			Target:   counterPartyKey,
			Sequence: sequence,
		},
		SwapMessage: swapInit,
		Stage:       action.SWAP_MATCHING,
	}

	return SignAndPack(action.Transaction(swap))
}

type ExSendArguments struct {
	SenderId        string
	ReceiverId      string
	SenderAddress   string
	ReceiverAddress string
	Currency        string
	Amount          string
	Gas             string
	Fee             string
	Chain           string
	ExGas           string
	ExFee           string
}

func CreateExSendRequest(args *ExSendArguments) []byte {
	conv := convert.NewConvert()

	partyKey := GetAccountKey(args.SenderId)
	cpartyKey := GetAccountKey(args.ReceiverId)

	fee := conv.GetCoin(args.Fee, "OLT")
	gas := conv.GetCoin(args.Gas, "OLT")
	amount := conv.GetCoin(args.Amount, args.Currency)
	chain := conv.GetChainFromCurrency(args.Chain)

	sender := GetCurrencyAddress(conv.GetCurrency(args.Currency), args.SenderId)
	reciever := GetCurrencyAddress(conv.GetCurrency(args.Currency), args.ReceiverId)
	signers := action.GetSigners(sender)

	sequence := GetSequenceNumber(partyKey)

	exSend := &action.ExternalSend{
		Base: action.Base{
			Type:     action.EXTERNAL_SEND,
			ChainId:  app.ChainId,
			Signers:  signers,
			Owner:    partyKey,
			Target:   cpartyKey,
			Sequence: sequence,
		},
		Gas:      gas,
		Fee:      fee,
		Chain:    chain,
		Sender:   string(sender),
		Receiver: string(reciever),
		Amount:   amount,
	}

	return SignAndPack(exSend)
}
