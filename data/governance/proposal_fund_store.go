package governance

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/Oneledger/protocol/data/balance"
	"github.com/Oneledger/protocol/data/keys"
	"github.com/Oneledger/protocol/serialize"
	"github.com/Oneledger/protocol/storage"
)

type ProposalFundStore struct {
	State  *storage.State
	prefix []byte
}

func (pf *ProposalFundStore) set(key storage.StoreKey, amt balance.Amount) error {
	dat, err := serialize.GetSerializer(serialize.PERSISTENT).Serialize(amt)
	if err != nil {
		return errors.Wrap(err, errorSerialization)
	}
	prefixed := append(pf.prefix, key...)
	err = pf.State.Set(prefixed, dat)
	return errors.Wrap(err, errorSettingRecord)
}

func (pf *ProposalFundStore) get(key storage.StoreKey) (amt *balance.Amount, err error) {
	prefixed := append(pf.prefix, storage.StoreKey(key)...)
	dat, err := pf.State.Get(prefixed)
	//fmt.Println("dat :", dat, "err", err)
	if err != nil {
		return nil, errors.Wrap(err, errorGettingRecord)
	}
	amt = balance.NewAmount(0)
	if len(dat) == 0 {
		return
	}
	err = serialize.GetSerializer(serialize.PERSISTENT).Deserialize(dat, amt)
	if err != nil {
		err = errors.Wrap(err, errorDeSerialization)
	}
	return
}

func (pf *ProposalFundStore) delete(key storage.StoreKey) (bool, error) {
	prefixed := append(pf.prefix, key...)
	res, err := pf.State.Delete(prefixed)
	if err != nil {
		return false, errors.Wrap(err, errorDeletingRecord)
	}
	return res, err
}

func (pf *ProposalFundStore) iterate(fn func(proposalID ProposalID, addr keys.Address, amt *balance.Amount) bool) bool {
	return pf.State.IterateRange(
		pf.prefix,
		storage.Rangefix(string(pf.prefix)),
		true,
		func(key, value []byte) bool {
			amt := balance.NewAmount(0)
			err := serialize.GetSerializer(serialize.PERSISTENT).Deserialize(value, amt)
			if err != nil {
				return true
			}
			arr := strings.Split(string(key), storage.DB_PREFIX)
			proposalID := arr[1]
			var fundingAddress keys.Address = nil
			// do not unmarshal address if it's total funds record, since its funding address is nil
			if arr[len(arr)-1] != storage.TOTALFUNDS {
				fundingAddress = keys.Address(arr[len(arr)-1])
				err = fundingAddress.UnmarshalText([]byte(arr[len(arr)-1]))
				if err != nil {
					fmt.Println("Error Unmarshalling ", err)
					return true
				}
			}
			return fn(ProposalID(proposalID), fundingAddress, amt)
		},
	)
}

func (pf *ProposalFundStore) WithState(state *storage.State) *ProposalFundStore {
	pf.State = state
	return pf
}

// Store Function Called my external Layers
func NewProposalFundStore(prefix string, state *storage.State) *ProposalFundStore {
	return &ProposalFundStore{
		State:  state,
		prefix: storage.Prefix(prefix),
	}
}

func (pf *ProposalFundStore) GetFundsForProposalID(id ProposalID, fn func(proposalID ProposalID, fundingAddr keys.Address, amt *balance.Amount) ProposalFund) []ProposalFund {
	var foundProposals []ProposalFund
	pf.iterate(func(proposalID ProposalID, fundingAddr keys.Address, amt *balance.Amount) bool {
		//ignore total funds record(its fundingAddr field is nil)
		if proposalID == id && fundingAddr != nil {
			foundProposals = append(foundProposals, fn(proposalID, fundingAddr, amt))
		}
		return false
	})
	return foundProposals
}

func (pf *ProposalFundStore) GetProposalsForFunder(funderAddress keys.Address, fn func(proposalID ProposalID, fundingAddr keys.Address, amt *balance.Amount) ProposalFund) []ProposalFund {
	var foundProposals []ProposalFund
	pf.iterate(func(proposalID ProposalID, fundingAddr keys.Address, amt *balance.Amount) bool {
		if bytes.Equal(funderAddress, fundingAddr) {
			foundProposals = append(foundProposals, fn(proposalID, fundingAddr, amt))
		}
		return false
	})
	return foundProposals
}

func (store *ProposalFundStore) IsFundedByFunder(id ProposalID, funder keys.Address) bool {
	haveFunderAddress := false
	store.GetFundsForProposalID(id, func(proposalID ProposalID, fundingAddr keys.Address, amt *balance.Amount) ProposalFund {
		//fmt.Println("fundingAddr: ", fundingAddr)
		//fmt.Println("funder: ", funder)
		if fundingAddr.Equal(funder) {
			haveFunderAddress = true
		}
		return ProposalFund{}
	})
	return haveFunderAddress
}

func (pf *ProposalFundStore) GetCurrentFundsForProposal(proposalID ProposalID) (*balance.Amount, error) {
	key := totalFundsKey(proposalID)
	funds, err := pf.getTotalFunds(key)
	if err != nil {
		return nil, errors.Wrap(err, errorGettingRecord)
	}
	return funds, nil
}

func totalFundsKey(proposalId ProposalID) storage.StoreKey {
	key := storage.StoreKey(string(proposalId) + storage.DB_PREFIX + storage.TOTALFUNDS)
	return key
}

func (pf *ProposalFundStore) getTotalFunds(key storage.StoreKey) (*balance.Amount, error) {
	amt, err := pf.get(key)
	if err != nil {
		return nil, errors.Wrap(err, errorGettingRecord)
	}
	return amt, nil
}

func (pf *ProposalFundStore) setTotalFunds(key storage.StoreKey, amount balance.Amount) error {
	err := pf.set(key, amount)
	if err != nil {
		return errors.Wrap(err, errorSettingRecord)
	}
	return nil
}

func (pf *ProposalFundStore) AddFunds(proposalId ProposalID, fundingAddress keys.Address, amount *balance.Amount) error {
	key := storage.StoreKey(string(proposalId) + storage.DB_PREFIX + fundingAddress.String())
	keyTotal := totalFundsKey(proposalId)
	amt, err := pf.get(key)
	if err != nil {
		return errors.Wrap(err, errorGettingRecord)
	}
	amtTotal, err := pf.getTotalFunds(keyTotal)
	if err != nil {
		return errors.Wrap(err, errorGettingRecord)
	}

	err = pf.set(key, *amt.Plus(*amount))
	if err != nil {
		return err
	}
	err = pf.setTotalFunds(keyTotal, *amtTotal.Plus(*amount))
	if err != nil {
		return err
	}
	return nil
}

func (pf *ProposalFundStore) DeductFunds(proposalId ProposalID, fundingAddress keys.Address, amount *balance.Amount) error {
	key := storage.StoreKey(string(proposalId) + storage.DB_PREFIX + fundingAddress.String())
	keyTotal := totalFundsKey(proposalId)
	amt, err := pf.get(key)
	if err != nil {
		return errors.Wrap(err, errorGettingRecord)
	}
	amtTotal, err := pf.getTotalFunds(keyTotal)
	if err != nil {
		return errors.Wrap(err, errorGettingRecord)
	}
	result, err := amt.Minus(*amount)
	if err != nil {
		return errors.Wrap(err, errorGettingRecord)
	}
	resultTotal, err := amtTotal.Minus(*amount)
	if err != nil {
		return errors.Wrap(err, errorGettingRecord)
	}
	err = pf.set(key, *result)
	if err != nil {
		return err
	}
	err = pf.setTotalFunds(keyTotal, *resultTotal)
	if err != nil {
		return err
	}
	return nil
}

func (pf *ProposalFundStore) DeleteFunds(proposalId ProposalID, fundingAddress keys.Address) (bool, error) {
	key := storage.StoreKey(string(proposalId) + storage.DB_PREFIX + fundingAddress.String())
	keyTotal := totalFundsKey(proposalId)
	amt, err := pf.get(key)
	if err != nil {
		return false, errors.Wrap(err, errorGettingRecord)
	}
	amtTotal, err := pf.getTotalFunds(keyTotal)
	if err != nil {
		return false, errors.Wrap(err, errorGettingRecord)
	}
	ok, err := pf.delete(key)
	if err != nil {
		return false, errors.Wrap(err, errorDeletingRecord)
	}
	resultTotal, err := amtTotal.Minus(*amt)
	if err != nil {
		return false, errors.Wrap(err, errorDeletingRecord)
	}
	err = pf.setTotalFunds(keyTotal, *resultTotal)
	if err != nil {
		return false, err
	}

	return ok, nil
}
