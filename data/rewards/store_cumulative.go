package rewards

import (
	"github.com/Oneledger/protocol/data/balance"
	"github.com/Oneledger/protocol/data/keys"
	"github.com/Oneledger/protocol/serialize"
	"github.com/Oneledger/protocol/storage"
	"github.com/pkg/errors"
	tmstore "github.com/tendermint/tendermint/store"
)

type RewardCumulativeStore struct {
	state  *storage.State
	szlr   serialize.Serializer
	prefix []byte

	blockStore    *tmstore.BlockStore
	rewardOptions *Options
}

func NewRewardCumulativeStore(prefix string, state *storage.State) *RewardCumulativeStore {
	return &RewardCumulativeStore{
		state:  state,
		prefix: storage.Prefix(prefix),
		szlr:   serialize.GetSerializer(serialize.PERSISTENT),
	}
}

func (rws *RewardCumulativeStore) WithState(state *storage.State) *RewardCumulativeStore {
	rws.state = state
	return rws
}

// Pull a combined block rewards from total supply for all voting validators for given block height
func (rws *RewardCumulativeStore) PullRewards(height int64, poolAmt *balance.Amount) (amount *balance.Amount, burnedout bool, year int, err error) {
	// get year distributed amount till now
	yearRewards, err := rws.GetYearDistributedRewards()
	if err != nil {
		return
	}

	// calculate reward for each block
	calc := NewRewardCalculator(height, yearRewards, rws.blockStore, rws.rewardOptions)
	amount, burnedout, year, err = calc.Calculate()
	if err != nil {
		return
	}

	// calculate burnout rate
	if burnedout && poolAmt.LessThan(*amount) {
		*amount = *poolAmt
	}
	return
}

// Deduct actual distributed block rewards from total/year supply
func (rws *RewardCumulativeStore) ConsumeRewards(consumed *balance.Amount, burnedout bool, year int) error {
	// accumulates total distributed rewards
	err := rws.addTotalDistributedRewards(consumed)
	if err != nil {
		return err
	}

	// accumulates year distributed rewards
	if burnedout {
		yearRewards, err := rws.GetYearDistributedRewards()
		if err != nil {
			return err
		}
		err = rws.addYearDistributedRewards(yearRewards, year, consumed)
	}
	return err
}

// Get total distributed rewards till now.
func (rws *RewardCumulativeStore) GetTotalDistributedRewards() (amt *balance.Amount, err error) {
	key := rws.getTotalDistributedKey()
	amt, err = rws.get(key)
	return
}

// Get a list of each year's distributed rewards till now
func (rws *RewardCumulativeStore) GetYearDistributedRewards() (yearRewards []*YearReward, err error) {
	key := rws.getYearDistributedKey()
	if !rws.state.Exists(key) {
		yearRewards, err = rws.initYearRewards(key)
		return
	}
	yearRewards, err = rws.getYearRewards(key)
	return
}

// Get matured rewards balance, the widrawable rewards, till now.
func (rws *RewardCumulativeStore) GetMaturedBalance(validator keys.Address) (amt *balance.Amount, err error) {
	key := rws.getBalanceKey(validator)
	amt, err = rws.get(key)
	return
}

func (rws *RewardCumulativeStore) IterateMaturedBalances(fn func(validator keys.Address, amt *balance.Amount) bool) (stopped bool) {
	return rws.iterate("balance_", fn)
}

// Add an 'amount' of matured rewards to rewards balance
func (rws *RewardCumulativeStore) AddMaturedBalance(validator keys.Address, amount *balance.Amount) error {
	key := rws.getBalanceKey(validator)
	amt, err := rws.get(key)
	if err != nil {
		return err
	}

	err = rws.set(key, amt.Plus(*amount))
	return err
}

// Get total matured rewards till now, including withdrawn amount. This number is calculated on the fly
func (rws *RewardCumulativeStore) GetMaturedRewards(validator keys.Address) (amt *balance.Amount, err error) {
	keyBalance := rws.getBalanceKey(validator)
	amtBalance, err := rws.get(keyBalance)
	if err != nil {
		return
	}

	keyWithdrawn := rws.getWithdrawnKey(validator)
	amtWithdrawn, err := rws.get(keyWithdrawn)
	if err != nil {
		return
	}

	amt = amtBalance.Plus(*amtWithdrawn)
	return
}

// Get total rewards withdrawn till now
func (rws *RewardCumulativeStore) GetWithdrawnRewards(validator keys.Address) (amt *balance.Amount, err error) {
	key := rws.getWithdrawnKey(validator)
	amt, err = rws.get(key)
	return
}

func (rws *RewardCumulativeStore) IterateWithdrawnRewards(fn func(validator keys.Address, amt *balance.Amount) bool) (stopped bool) {
	return rws.iterate("withdrawn_", fn)
}

// Withdraw an 'amount' of rewards from rewards balance
func (rws *RewardCumulativeStore) WithdrawRewards(validator keys.Address, amount *balance.Amount) error {

	err := rws.minusRewardsBalance(validator, amount)
	if err != nil {
		return errors.Wrap(err, "Minus from Matured Balance")
	}
	err = rws.addWithdrawnRewards(validator, amount)
	if err != nil {
		return errors.Wrap(err, "Add to Withdraw Balance")
	}

	return nil
}

func (rws *RewardCumulativeStore) SetOptions(options *Options) {
	rws.rewardOptions = options
}

func (rws *RewardCumulativeStore) GetOptions() *Options {
	return rws.rewardOptions
}

func (rws *RewardCumulativeStore) SetBlockStore(blockStore *tmstore.BlockStore) {
	rws.blockStore = blockStore
}

//-----------------------------helper functions
//
// Set cumulative amount(s) by key
func (rws *RewardCumulativeStore) set(key storage.StoreKey, amts interface{}) error {
	dat, err := rws.szlr.Serialize(amts)
	if err != nil {
		return err
	}
	err = rws.state.Set(key, dat)
	return err
}

// Get cumulative amount by key
func (rws *RewardCumulativeStore) get(key storage.StoreKey) (amt *balance.Amount, err error) {
	dat, err := rws.state.Get(key)
	if err != nil {
		return
	}
	amt = balance.NewAmount(0)
	if len(dat) == 0 {
		return
	}
	err = rws.szlr.Deserialize(dat, amt)
	return
}

// for matured balances and wirndraw amounts
func (rws *RewardCumulativeStore) iterate(subkey string, fn func(validator keys.Address, amt *balance.Amount) bool) (stopped bool) {
	prefix := append(rws.prefix, subkey...)
	return rws.state.IterateRange(
		prefix,
		storage.Rangefix(string(prefix)),
		true,
		func(key, value []byte) bool {
			amt := balance.NewAmount(0)
			err := rws.szlr.Deserialize(value, amt)
			if err != nil {
				logger.Error("failed to deserialize cumulative amount")
				return false
			}
			addr := key[len(prefix):]
			return fn(addr, amt)
		},
	)
}

// Get year rewards
func (rws *RewardCumulativeStore) getYearRewards(key storage.StoreKey) (rewards []*YearReward, err error) {
	dat, err := rws.state.Get(key)
	if err != nil {
		return
	}
	rewards = make([]*YearReward, 0)
	if len(dat) == 0 {
		err = YearRewardsMissing
		return
	}
	err = rws.szlr.Deserialize(dat, rewards)
	return
}

// Initialize each reward year's information
func (rws *RewardCumulativeStore) initYearRewards(key storage.StoreKey) (rewards []*YearReward, err error) {
	tStart := rws.blockStore.LoadBlockMeta(1).Header.Time.UTC()
	numofYears := len(rws.rewardOptions.YearBlockRewardShares)

	// calculate each year's start/close time
	rewards = make([]*YearReward, 0)
	for i := 0; i < numofYears; i++ {
		tClose := tStart.AddDate(1, 0, 0).UTC()
		reward := &YearReward{
			StartTime:   tStart,
			CloseTime:   tClose,
			Distributed: balance.NewAmount(0),
		}
		logger.Infof("Initial year-%v [start: %s], [close: %s]", i+1, tStart, tClose)
		tStart = tClose
		rewards = append(rewards, reward)
	}

	// save to DB
	err = rws.set(key, rewards)
	return
}

// Key for total distributed rewards
func (rws *RewardCumulativeStore) getTotalDistributedKey() []byte {
	key := string(rws.prefix) + "tdist"
	return storage.StoreKey(key)
}

// Key for each year's distributed rewards
func (rws *RewardCumulativeStore) getYearDistributedKey() []byte {
	key := string(rws.prefix) + "ydist"
	return storage.StoreKey(key)
}

// Key for balance
func (rws *RewardCumulativeStore) getBalanceKey(validator keys.Address) []byte {
	key := string(rws.prefix) + "balance" + storage.DB_PREFIX + validator.String()
	return storage.StoreKey(key)
}

// Key for withdrawn
func (rws *RewardCumulativeStore) getWithdrawnKey(validator keys.Address) []byte {
	key := string(rws.prefix) + "withdrawn" + storage.DB_PREFIX + validator.String()
	return storage.StoreKey(key)
}

// Add to total distributed rewards
func (rws *RewardCumulativeStore) addTotalDistributedRewards(amount *balance.Amount) error {
	key := rws.getTotalDistributedKey()
	amt, err := rws.get(key)
	if err != nil {
		return err
	}

	err = rws.set(key, amt.Plus(*amount))
	return err
}

// Add to total year distributed rewards
func (rws *RewardCumulativeStore) addYearDistributedRewards(yearRewards []*YearReward, year int, amount *balance.Amount) error {
	key := rws.getYearDistributedKey()
	yearDist := yearRewards[year].Distributed
	yearRewards[year].Distributed = yearDist.Plus(*amount)

	err := rws.set(key, yearRewards)
	return err
}

// Deducts an 'amount' of rewards from rewards balance
func (rws *RewardCumulativeStore) minusRewardsBalance(validator keys.Address, amount *balance.Amount) error {
	key := rws.getBalanceKey(validator)
	amt, err := rws.get(key)
	if err != nil {
		return err
	}

	result, err := amt.Minus(*amount)
	if err != nil {
		return err
	}

	err = rws.set(key, result)
	return err
}

// Add to total rewards withdrawn till now
func (rws *RewardCumulativeStore) addWithdrawnRewards(validator keys.Address, amount *balance.Amount) error {
	key := rws.getWithdrawnKey(validator)
	amt, err := rws.get(key)
	if err != nil {
		return err
	}

	err = rws.set(key, amt.Plus(*amount))
	return err
}

//-----------------------------Dump/Load chain state
//
type RewardAmount struct {
	Address keys.Address    `json:"address"`
	Amount  *balance.Amount `json:"amount"`
}

type RewardCumuState struct {
	TotalDistributed *balance.Amount `json:"totalDistributed"`
	YearsDistributed []*YearReward   `json:"yearsDistributed"`
	MaturedBalances  []RewardAmount  `json:"maturedBalances"`
	WithdrawnAmounts []RewardAmount  `json:"withdrawnAmounts"`
}

func (rws *RewardCumulativeStore) dumpState() (state *RewardCumuState, err error) {
	// dump total distributed rewards
	state = &RewardCumuState{}
	state.TotalDistributed, err = rws.GetTotalDistributedRewards()
	if err != nil {
		return
	}
	// dump each year's distributed rewards
	state.YearsDistributed, err = rws.GetYearDistributedRewards()
	if err != nil {
		return
	}
	// dump each validator's matured balance
	rws.iterate("balance_", func(addr keys.Address, amt *balance.Amount) bool {
		matured := RewardAmount{
			Address: addr,
			Amount:  amt,
		}
		state.MaturedBalances = append(state.MaturedBalances, matured)
		return false
	})
	// dump each validator's total withdrawn rewards
	rws.iterate("withdrawn_", func(addr keys.Address, amt *balance.Amount) bool {
		draw := RewardAmount{
			Address: addr,
			Amount:  amt,
		}
		state.WithdrawnAmounts = append(state.WithdrawnAmounts, draw)
		return false
	})
	return
}

func (rws *RewardCumulativeStore) loadState(state *RewardCumuState) (err error) {
	err = rws.addTotalDistributedRewards(state.TotalDistributed)
	if err != nil {
		return
	}
	if len(state.YearsDistributed) == len(rws.rewardOptions.YearBlockRewardShares) {
		key := rws.getYearDistributedKey()
		err = rws.set(key, state.YearsDistributed)
		if err != nil {
			return errors.Wrap(err, "failed to load initial year distributed amounts")
		}
	}
	for _, matured := range state.MaturedBalances {
		err = rws.AddMaturedBalance(matured.Address, matured.Amount)
		if err != nil {
			return errors.Wrap(err, "failed to load initial matured balance")
		}
	}
	for _, draw := range state.WithdrawnAmounts {
		err = rws.addWithdrawnRewards(draw.Address, draw.Amount)
		if err != nil {
			return errors.Wrap(err, "failed to load initial withdrawn amount")
		}
	}
	return nil
}
