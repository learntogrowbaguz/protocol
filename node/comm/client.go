/*
	Copyright 2017-2018 OneLedger

	Cover over the Tendermint client handling.

	TODO: Make this generic to handle HTTP and local clients
*/
package comm

import (
	"github.com/Oneledger/protocol/node/serial"
	"github.com/Oneledger/protocol/node/status"
	"reflect"

	"github.com/Oneledger/protocol/node/global"
	"github.com/Oneledger/protocol/node/log"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

type ClientContext struct {
	Client rpcclient.Client
	Async  bool
}

type ClientInterface interface {
	BroadcastTxSync(packet []byte) (*ctypes.ResultBroadcastTx, status.Code)
	BroadcastTxAsync(packet []byte) (*ctypes.ResultBroadcastTx, status.Code)
	BroadcastTxCommit(packet []byte) (*ctypes.ResultBroadcastTxCommit, status.Code)
	ABCIQuery(path string, packet []byte) (*ctypes.ResultABCIQuery, status.Code)
	Tx(hash []byte, prove bool) (*ctypes.ResultTx, error)
	TxSearch(query string, prove bool, page, perPage int) (*ctypes.ResultTxSearch, error)
	Block(height *int64) (*ctypes.ResultBlock, error)
	Status() (*ctypes.ResultStatus, error)
	Start() error
	IsRunning() bool
}

func (client ClientContext) BroadcastTxSync(packet []byte) (*ctypes.ResultBroadcastTx, status.Code) {

	if len(packet) < 1 {
		log.Debug("Empty Transaction")
		return nil, status.MISSING_DATA
	}

	log.Debug("Start Synced Broadcast", "packet", packet)

	result, err := client.Client.BroadcastTxSync(packet)

	StopClient()

	if err != nil {
		log.Error("Error", "err", err)
		return nil, status.EXECUTE_ERROR
	}

	log.Debug("Finished Synced Broadcast", "packet", packet, "result", result)

	return result, status.SUCCESS
}

func (client ClientContext) BroadcastTxAsync(packet []byte) (*ctypes.ResultBroadcastTx, status.Code) {
	if len(packet) < 1 {
		log.Debug("Empty Transaction")
		return nil, status.MISSING_DATA
	}

	result, err := client.Client.BroadcastTxAsync(packet)

	// @todo Do we need to stop Client?
	StopClient()

	if err != nil {
		log.Error("Broadcast Error", "err", err)
		return nil, status.EXECUTE_ERROR
	}

	log.Debug("Broadcast", "packet", packet, "result", result)

	return result, status.SUCCESS
}

func (client ClientContext) BroadcastTxCommit(packet []byte) (*ctypes.ResultBroadcastTxCommit, status.Code) {
	if len(packet) < 1 {
		log.Debug("Empty Transaction")
		return nil, status.MISSING_DATA
	}

	log.Debug("Start Synced Broadcast", "packet", packet)

	result, err := client.Client.BroadcastTxCommit(packet)

	// @todo Do we need to stop Client?
	StopClient()

	if err != nil {
		log.Error("Error", "err", err)
		return nil, status.EXECUTE_ERROR
	}

	log.Debug("Finished Synced Broadcast", "packet", packet, "result", result)

	return result, status.SUCCESS
}

func (client ClientContext) ABCIQuery(path string, packet []byte) (*ctypes.ResultABCIQuery, status.Code) {

	if len(path) < 1 {
		log.Debug("Empty Query Path")
		return nil, status.MISSING_DATA
	}

	var response *ctypes.ResultABCIQuery
	var err error

	response, err = client.Client.ABCIQuery(path, packet)

	// @todo Do we need to stop Client?
	StopClient()

	if err != nil {
		log.Debug("ABCi Query Error", "path", path, "err", err)
		return nil, status.EXECUTE_ERROR
	}

	if response == nil {
		log.Debug("response is empty")
		return nil, status.EXECUTE_ERROR
	}

	result, err := serial.Deserialize(response.Response.Value, response, serial.CLIENT)

	if err != nil {
		log.Error("Failed to deserialize Query:", "response", response.Response.Value)
		return nil, status.BAD_VALUE
	}

	return result.(*ctypes.ResultABCIQuery), status.SUCCESS
}

func (client ClientContext) Tx(hash []byte, prove bool) (*ctypes.ResultTx, error) {
	return client.Client.Tx(hash, prove)
}

func (client ClientContext) TxSearch(query string, prove bool, page, perPage int) (*ctypes.ResultTxSearch, error) {
	return client.Client.TxSearch(query, prove, page, perPage)
}

func (client ClientContext) Block(height *int64) (*ctypes.ResultBlock, error) {
	return client.Client.Block(height)
}

func (client ClientContext) Status() (*ctypes.ResultStatus, error) {
	return client.Client.Status()
}

func (client ClientContext) Start() error {
	return client.Client.Start()
}

func (client ClientContext) IsRunning() bool {
	return client.Client.IsRunning()
}

var cachedClient ClientInterface

var transactionPathsMap = map[string]bool{
	"/applyValidators":     true,
	"/createExSendRequest": true,
	"/createSendRequest":   true,
	"/createMintRequest":   true,
	"/createSwapRequest":   true,
	"/nodeName":            true,
	"/signTransaction":     true,
}

// HTTP interface, allows Broadcast?
// TODO: Want to switch client type, based on config or cli args.
func GetClient() (client ClientInterface) {

	var rpc rpcclient.Client

	defer func() {
		if r := recover(); r != nil {
			log.Debug("Ignoring Client Panic", "r", r)
			client = nil
		}
	}()

	if cachedClient != nil {
		return cachedClient
	}

	if global.Current.ConsensusNode != nil {
		log.Debug("Using local ConsensusNode ABCI Client")
		rpc = rpcclient.NewLocal(global.Current.ConsensusNode)

	} else {
		log.Debug("Using new HTTP ABCI Client")
		rpc = rpcclient.NewHTTP(global.Current.Config.Network.RPCAddress, "/websocket")
	}

	client = ClientContext{
		Client: rpc,
		Async:  false,
	}

	if _, err := client.Status(); err == nil {
		log.Debug("Client is running")
		cachedClient = client
		return
	}

	if err := cachedClient.Start(); err != nil {
		log.Fatal("Client is unavailable", "address", global.Current.Config.Network.RPCAddress)
		client = nil
		return
	}

	return
}

func StopClient() {
	if cachedClient != nil && cachedClient.IsRunning() {
		//cachedClient.Stop()
	}
}

func IsError(result interface{}) *string {
	if reflect.TypeOf(result).Kind() == reflect.String {
		final := result.(string)
		return &final
	}
	return nil
}

// Send a very specific query
func Query(path string, packet []byte) interface{} {
	if len(path) < 1 {
		log.Debug("Empty Query Path")
		return nil
	}

	var response *ctypes.ResultABCIQuery
	var err error

	client := GetClient()
	if client == nil {
		log.Debug("Client Unavailable")
		return nil
	}

	response, _ = client.ABCIQuery(path, packet)
	StopClient()

	if err != nil {
		log.Debug("ABCi Query Error", "path", path, "err", err)
		return nil
	}
	//if response != nil {
	//	break
	//}
	//time.Sleep(2 * time.Second)
	//}

	if response == nil {
		//return "No results for " + path + " and " + string(packet)
		log.Debug("response is empty")
		return nil
	}

	var result interface{}

	_, isTransactionPath := transactionPathsMap[path]
	if isTransactionPath {
		// we continue to use old serializer for query handlers who
		// return transaction interface, which is yet to be moved to
		// the new serializer
		var proto interface{}
		result, err = serial.Deserialize(response.Response.Value, proto, serial.CLIENT)
	} else {

		err = clSerializer.Deserialize(response.Response.Value, &result)
	}

	if err != nil {
		log.Error("Failed to deserialize Query:", "response", response.Response.Value)
		return nil
	}

	return result
}

func Tx(hash []byte, prove bool) (res *ctypes.ResultTx) {
	client := GetClient()

	result, err := client.Tx(hash, prove)
	if err != nil {
		log.Error("TxSearch Error", "err", err)
		return nil
	}

	log.Debug("TxSearch", "hash", hash, "prove", prove, "result", result)

	return result
}

func Search(query string, prove bool, page, perPage int) (res *ctypes.ResultTxSearch) {
	client := GetClient()

	result, err := client.TxSearch(query, prove, page, perPage)
	if err != nil {
		log.Error("TxSearch Error", "err", err)
		return nil
	}

	log.Debug("TxSearch", "query", query, "prove", prove, "result", result)

	return result
}

func Block(height int64) (res *ctypes.ResultBlock) {
	client := GetClient()

	// Pass nil if given 0 to return the latest block
	var h *int64
	if height != 0 {
		h = &height
	}
	result, err := client.Block(h)
	if err != nil {
		return nil
	}
	return result
}
