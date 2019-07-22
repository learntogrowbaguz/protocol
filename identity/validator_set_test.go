package identity

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"testing"

	"github.com/Oneledger/protocol/utils"
	"github.com/pkg/errors"

	"github.com/Oneledger/protocol/config"
	"github.com/Oneledger/protocol/data/balance"
	"github.com/Oneledger/protocol/data/chain"
	"github.com/Oneledger/protocol/data/keys"
	"github.com/stretchr/testify/assert"
	"github.com/tendermint/tendermint/abci/types"
)

func TestNewValidatorStore(t *testing.T) {
	vs, removePaths := setup()
	defer teardown(removePaths)
	if assert.NotEmpty(t, vs) {
		assert.Equal(t, 0, vs.queue.Len())
		assert.Equal(t, 0, len(vs.byzantine))
		assert.Equal(t, keys.Address(nil), vs.proposer)
	}
}

// global setup
func setup() (*ValidatorStore, []string) {
	testDBPath := "test_dbpath"
	testdbType := "test_dbtype"
	vs := NewValidatorStore(config.Server{}, testDBPath, testdbType)
	removePaths := []string{testDBPath}
	return vs, removePaths
}

// remove test_db dir after test
func teardown(dbPaths []string) {
	for _, v := range dbPaths {
		err := os.RemoveAll(v)
		if err != nil {
			errors.New("Remove test db file error")
		}
	}
}

func prepareStake(address string) Stake {
	addr, _ := hex.DecodeString(address)
	currency := balance.Currency{
		Name:  "VT",
		Chain: chain.Type(1),
	}
	coin := balance.Coin{
		Currency: currency,
		Amount:   balance.NewAmount(0),
	}
	pubkey := keys.PublicKey{
		KeyType: keys.ED25519,
		Data:    nil,
	}
	apply := Stake{
		ValidatorAddress: addr,
		StakeAddress:     addr,
		Pubkey:           pubkey,
		Name:             "test_name",
		Amount:           coin,
	}
	return apply
}

func prepareUnstake(address string) Unstake {
	validatorAddr, _ := hex.DecodeString(address)
	currency := balance.Currency{
		Name:  "VT",
		Chain: chain.Type(1),
	}
	coin := balance.Coin{
		Currency: currency,
		Amount:   balance.NewAmount(0),
	}
	unstake := Unstake{
		Address: validatorAddr,
		Amount:  coin,
	}
	return unstake
}

func setupForInit(pubKeyType string, pubKeyData []byte, currencyName string, power int64) (types.RequestInitChain, *balance.CurrencyList) {
	// prepare for request
	validatorUpdates := make([]types.ValidatorUpdate, 0)
	ValidatorUpdate := types.ValidatorUpdate{
		PubKey: types.PubKey{Type: pubKeyType, Data: pubKeyData},
		Power:  power,
	}
	validatorUpdates = append(validatorUpdates, ValidatorUpdate)
	req := types.RequestInitChain{
		Validators: validatorUpdates,
	}
	// prepare for currencies
	currencies := balance.NewCurrencyList()
	currency := balance.Currency{
		Name: currencyName,
	}
	currencies.Register(currency)
	// initial a validatorStore and call Init()
	return req, currencies
}

func TestValidatorStore_Init(t *testing.T) {
	t.Run("run with invalid currency type, should return token not registered error", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		req, currencies := setupForInit("", []byte(""), "VTT", 0)
		_, err := vs.Init(req, currencies)

		assert.EqualError(t, err, "stake token not registered")

	})
	t.Run("run with invalid pubkey type, should return invalid key algorithm error", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		req, currencies := setupForInit("ed25520", []byte(""), "VT", 0)
		_, err := vs.Init(req, currencies)
		assert.EqualError(t, err, "invalid pubkey type: provided invalid key algorithm")
	})
	t.Run("add initial validator, should return no error", func(t *testing.T) {
		pubKeyData, _ := base64.StdEncoding.DecodeString("lLkWE3WfWrtqy2qiKw+dcD4mpQ2NW+K6ldzin4o1b9Q=")
		vs, removePaths := setup()
		defer teardown(removePaths)
		req, currencies := setupForInit("ed25519", pubKeyData, "VT", 100)
		_, err := vs.Init(req, currencies)
		assert.NoError(t, err)
	})
}

func setupForSet() (types.RequestBeginBlock, types.Validator, []types.VoteInfo, Stake) {
	address := "F3FC12B8442A3FF95156331F3246AD9EFE232947"
	addr, _ := hex.DecodeString(address)

	validator := types.Validator{
		Address: addr,
		Power:   500,
	}
	voteInfo := make([]types.VoteInfo, 0)

	evinstance := types.Evidence{
		Type:      "test_type",
		Validator: validator,
		Height:    3,
	}
	ev := make([]types.Evidence, 0)
	ev = append(ev, evinstance)

	// prepare for request
	req := types.RequestBeginBlock{
		LastCommitInfo: types.LastCommitInfo{
			Votes: voteInfo,
		},
		Header:              types.Header{ProposerAddress: addr, Height: 1},
		ByzantineValidators: ev,
	}
	return req, validator, voteInfo, prepareStake(address)
}

func TestValidatorStore_Set(t *testing.T) {
	t.Run("update validator set, should return an error", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		req, validator, voteInfo, _ := setupForSet()
		vi := types.VoteInfo{
			Validator:       validator,
			SignedLastBlock: true,
		}
		voteInfo = append(voteInfo, vi)
		req.LastCommitInfo.Votes = voteInfo
		err := vs.Set(req)
		assert.Error(t, err, "validator set not match to last commit")
	})
	t.Run("update validator set successfully with valid stake", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		req2, _, _, stake := setupForSet()
		err := vs.HandleStake(stake)
		assert.Nil(t, err)
		vs.Commit()
		err = vs.Set(req2)
		assert.Nil(t, err)
	})
}

func TestValidatorStore_GetValidatorSet(t *testing.T) {
	vs, removePaths := setup()
	defer teardown(removePaths)
	validatorSet, _ := vs.GetValidatorSet()
	assert.Empty(t, validatorSet)
}

func setupForHandleStake() Stake {
	address := "f529ec288fbd333895cfa1aca272950064f1dbc1"
	return prepareStake(address)
}

func TestValidatorStore_HandleStake(t *testing.T) {
	vs, removePaths := setup()
	defer teardown(removePaths)
	apply := setupForHandleStake()

	vaList, _ := vs.GetValidatorSet()
	assert.Empty(t, vaList)

	assert.NoError(t, vs.HandleStake(apply))

	vs.ChainState.Commit()

	assert.NoError(t, vs.HandleStake(apply))

	vaList, _ = vs.GetValidatorSet()
	assert.NotEmpty(t, vaList)
}

func setupForUnHandleStake() (Unstake, Stake) {
	address := "f529ec288fbd333895cfa1aca272950064f1dbc1"
	return prepareUnstake(address), prepareStake(address)
}

func TestValidatorStore_HandleUnstake(t *testing.T) {
	t.Run("check chainstate exist, should return an error", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		unstake, _ := setupForUnHandleStake()
		err := vs.HandleUnstake(unstake)
		assert.Error(t, err)
	})
	t.Run("check chainstate get, should return no error", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		unstake, stake := setupForUnHandleStake()
		vs.HandleStake(stake)
		vs.Commit()
		err := vs.HandleUnstake(unstake)
		assert.NoError(t, err)
	})
	t.Run("unstake with invalid currency type, should return error", func(t *testing.T) {
		vs, removePaths := setup()
		defer teardown(removePaths)
		unstake, stake := setupForUnHandleStake()
		err := vs.HandleStake(stake)
		assert.Nil(t, err)
		vs.Commit()

		// invalid currency type
		currency := balance.Currency{
			Name:  "ABC",
			Chain: chain.Type(1),
		}
		coin := balance.Coin{
			Currency: currency,
			Amount:   balance.NewAmount(1000),
		}
		unstake.Amount = coin
		err = vs.HandleUnstake(unstake)
		assert.Error(t, err)
	})
}

func setupForGetEndBlockUpdate() (types.RequestEndBlock, Stake, Stake, []byte) {
	req := types.RequestEndBlock{
		Height: 2,
	}
	address := "f529ec288fbd333895cfa1aca272950064f1dbc1"
	validatorAddr, _ := hex.DecodeString(address)
	return req, prepareStake(address), prepareStake(""), validatorAddr
}

func TestValidatorStore_GetEndBlockUpdate(t *testing.T) {
	vs, removePaths := setup()
	defer teardown(removePaths)
	req, stake, stake1, validatorAddr := setupForGetEndBlockUpdate()

	// prepare for testing data
	vs.queue.PriorityQueue = make(utils.PriorityQueue, 0, 100)
	// valid validator test data
	queued := utils.NewQueued(validatorAddr, 0, 1)
	vs.queue.append(queued)
	vs.queue.Init()
	err := vs.HandleStake(stake)
	assert.Nil(t, err)
	vs.Commit()

	// invalid validator test data1
	queued1 := utils.NewQueued([]byte("nonsenceaddress"), 0, 1)
	vs.queue.append(queued1)
	vs.queue.Init()
	err = vs.HandleStake(stake1)
	assert.Nil(t, err)
	vs.Commit()

	validatorUpdates := vs.GetEndBlockUpdate(nil, req)
	if assert.NotEmpty(t, validatorUpdates) {
		assert.Len(t, validatorUpdates, 1)
	}
}

func TestValidatorStore_Commit(t *testing.T) {
	vs, removePaths := setup()
	defer teardown(removePaths)
	apply := setupForHandleStake()
	err := vs.HandleStake(apply)
	assert.Nil(t, err)
	result, index := vs.Commit()
	assert.NotEmpty(t, result)
	assert.Equal(t, int64(1), index)
}