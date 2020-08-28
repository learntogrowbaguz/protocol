package governance

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/libs/kv"

	"github.com/Oneledger/protocol/action"
	"github.com/Oneledger/protocol/action/helpers"
	"github.com/Oneledger/protocol/data/balance"
	"github.com/Oneledger/protocol/data/governance"
	"github.com/Oneledger/protocol/data/keys"
)

var _ action.Msg = &CreateProposal{}

type CreateProposal struct {
	ProposalID      governance.ProposalID   `json:"proposalId"`
	ProposalType    governance.ProposalType `json:"proposalType"`
	Headline        string                  `json:"proposalHeadline"`
	Description     string                  `json:"proposalDescription"`
	Proposer        keys.Address            `json:"proposerAddress"`
	InitialFunding  action.Amount           `json:"initialFunding"`
	FundingDeadline int64                   `json:"fundingDeadline"`
	FundingGoal     *balance.Amount         `json:"fundingGoal"`
	VotingDeadline  int64                   `json:"votingDeadline"`
	PassPercentage  int                     `json:"passPercentage"`
	ConfigUpdate    interface{}             `json:"configUpdate"`
}

func (c CreateProposal) Validate(ctx *action.Context, signedTx action.SignedTx) (bool, error) {
	createProposal := CreateProposal{}
	err := createProposal.Unmarshal(signedTx.Data)
	if err != nil {
		return false, errors.Wrap(action.ErrWrongTxType, err.Error())
	}

	//validate basic signature
	err = action.ValidateBasic(signedTx.RawBytes(), createProposal.Signers(), signedTx.Signatures)
	if err != nil {
		return false, err
	}
	feeOpt, err := ctx.GovernanceStore.GetFeeOption()
	if err != nil {
		return false, governance.ErrGetFeeOptions
	}
	err = action.ValidateFee(feeOpt, signedTx.Fee)
	if err != nil {
		return false, err
	}

	//options := ctx.ProposalMasterStore.Proposal.GetOptionsByType(createProposal.ProposalType)
	options, err := ctx.GovernanceStore.GetProposalOptionsByType(createProposal.ProposalType)
	if err != nil {
		return false, governance.ErrGetProposalOptions
	}

	// the currency should be OLT
	currency, ok := ctx.Currencies.GetCurrencyById(0)
	if !ok {
		panic("no default currency available in the network")
	}
	if currency.Name != createProposal.InitialFunding.Currency {
		return false, errors.Wrap(action.ErrInvalidAmount, createProposal.InitialFunding.String())
	}

	//Check if Proposal ID is valid
	if err = createProposal.ProposalID.Err(); err != nil {
		return false, governance.ErrInvalidProposalId
	}

	//Get Proposal options based on type.
	coin := createProposal.InitialFunding.ToCoin(ctx.Currencies)
	coinInit := coin.Currency.NewCoinFromAmount(*options.InitialFunding)
	coinGoal := coin.Currency.NewCoinFromAmount(*options.FundingGoal)

	//Check if initial funding is not less than minimum amount based on type.
	if coin.LessThanCoin(coinInit) {
		return false, errors.Wrap(action.ErrInvalidAmount, "Funding Less than initial funding")
	}

	//Check if initial funding is more than funding goal.
	if coinGoal.LessThanEqualCoin(coin) {
		return false, errors.Wrap(action.ErrInvalidAmount, "Funding More than Funding goal")
	}

	//Check if Proposal Type is valid
	switch createProposal.ProposalType {
	case governance.ProposalTypeGeneral:
	case governance.ProposalTypeCodeChange:
	case governance.ProposalTypeConfigUpdate:
	default:
		return false, governance.ErrInvalidProposalType
	}

	//Check if proposer address is valid oneLedger address
	err = createProposal.Proposer.Err()
	if err != nil {
		return false, errors.Wrap(action.ErrInvalidAddress, err.Error())
	}

	if len(createProposal.Description) == 0 {
		return false, governance.ErrInvalidProposalDesc
	}

	//Validate funding goal and pass percentage
	if !createProposal.FundingGoal.Equals(*options.FundingGoal) {
		return false, governance.ErrInvalidFundingGoal
	}

	if createProposal.PassPercentage != options.PassPercentage {
		return false, governance.ErrInvalidPassPercentage
	}

	//Validate voting height
	if createProposal.VotingDeadline-createProposal.FundingDeadline != options.VotingDeadline {
		return false, governance.ErrInvalidVotingDeadline
	}

	return true, nil
}

func (c CreateProposal) ProcessCheck(ctx *action.Context, tx action.RawTx) (bool, action.Response) {
	ctx.Logger.Detail("Processing CreateProposal Transaction for CheckTx", tx)
	return runTx(ctx, tx)
}

func (c CreateProposal) ProcessDeliver(ctx *action.Context, tx action.RawTx) (bool, action.Response) {
	ctx.Logger.Detail("Processing CreateProposal Transaction for DeliverTx", tx)
	return runTx(ctx, tx)
}

func (c CreateProposal) ProcessFee(ctx *action.Context, signedTx action.SignedTx, start action.Gas, size action.Gas) (bool, action.Response) {
	return action.BasicFeeHandling(ctx, signedTx, start, size, 1)
}

func runTx(ctx *action.Context, tx action.RawTx) (bool, action.Response) {
	createProposal := CreateProposal{}
	err := createProposal.Unmarshal(tx.Data)
	if err != nil {
		result := action.Response{
			Events: action.GetEvent(createProposal.Tags(), "create_proposal_failed_deserialize"),
			Log:    action.ErrWrongTxType.Wrap(err).Marshal(),
		}
		return false, result
	}

	//Validate funding height, this one is put here because in validate() we cannot always get valid ctx.Header
	if createProposal.FundingDeadline <= ctx.Header.Height {
		result := action.Response{
			Events: action.GetEvent(createProposal.Tags(), "create_proposal_invalid_funding_deadline"),
			Log:    governance.ErrInvalidFundingDeadline.Marshal(),
		}
		return false, result
	}
	//Get Proposal options based on type.
	//options, err := ctx.GovernanceStore.GetProposalOptionsByType(createProposal.ProposalType)
	//if err != nil {
	//	helpers.LogAndReturnFalse(ctx.Logger, governance.ErrGetProposalOptions, createProposal.Tags(), err)
	//}
	if createProposal.ProposalType == governance.ProposalTypeConfigUpdate {
		updates, ok := createProposal.ConfigUpdate.(map[string]interface{})
		if !ok {
			return helpers.LogAndReturnFalse(ctx.Logger, governance.ErrValidateGovState, createProposal.Tags(), errors.New("Invalide Update Object"))
		}
		for configUpdatePath, updateValue := range updates {
			updateFunc, ok := ctx.GovUpdate.GovernanceUpdateFunction[configUpdatePath]
			if !ok {
				return helpers.LogAndReturnFalse(ctx.Logger, governance.ErrValidateGovState, createProposal.Tags(), errors.New("Update "+configUpdatePath+" Not allowed"))
			}
			ok, err = updateFunc(updateValue, ctx, action.ValidateOnly)
			if err != nil || !ok {
				return helpers.LogAndReturnFalse(ctx.Logger, governance.ErrValidateGovState, createProposal.Tags(), err)
			}
		}

	}

	//Create Proposal and save to Proposal Store
	proposal := governance.NewProposal(
		createProposal.ProposalID,
		createProposal.ProposalType,
		createProposal.Description,
		createProposal.Headline,
		createProposal.Proposer,
		createProposal.FundingDeadline,
		createProposal.FundingGoal,
		createProposal.VotingDeadline,
		createProposal.PassPercentage,
		createProposal.ConfigUpdate)

	//Check if Proposal already exists
	if ctx.ProposalMasterStore.Proposal.Exists(proposal.ProposalID) {
		result := action.Response{
			Events: action.GetEvent(createProposal.Tags(), "create_proposal_already_exists"),
			Log:    governance.ErrProposalExists.Marshal(),
		}
		return false, result
	}

	//Add proposal to DB
	activeProposals := ctx.ProposalMasterStore.Proposal.WithPrefixType(governance.ProposalStateActive)
	err = activeProposals.Set(proposal)
	if err != nil {
		result := action.Response{
			Events: action.GetEvent(createProposal.Tags(), "create_proposal_failed"),
			Log:    governance.ErrAddingProposalToActiveStore.Wrap(err).Marshal(),
		}
		return false, result
	}

	//Deduct initial funding from proposer's address
	funds := createProposal.InitialFunding.ToCoin(ctx.Currencies)
	err = ctx.Balances.MinusFromAddress(createProposal.Proposer.Bytes(), funds)
	if err != nil {
		result := action.Response{
			Events: action.GetEvent(createProposal.Tags(), "create_proposal_deduction_failed"),
			Log:    governance.ErrDeductFunding.Wrap(err).Marshal(),
		}
		return false, result
	}

	//Add initial funds to the Proposal Fund store
	initialFunding := balance.NewAmountFromBigInt(createProposal.InitialFunding.Value.BigInt())
	err = ctx.ProposalMasterStore.ProposalFund.AddFunds(proposal.ProposalID, proposal.Proposer, initialFunding)
	if err != nil {
		result := action.Response{
			Events: action.GetEvent(createProposal.Tags(), "create_proposal_funding_failed"),
			Log:    governance.ErrAddFunding.Marshal(),
		}
		return false, result
	}

	result := action.Response{
		Events: action.GetEvent(createProposal.Tags(), "create_proposal_success"),
	}

	return true, result
}

func (c CreateProposal) Signers() []action.Address {
	return []action.Address{c.Proposer}
}

func (c CreateProposal) Type() action.Type {
	return action.PROPOSAL_CREATE
}

func (c CreateProposal) Tags() kv.Pairs {
	tags := make([]kv.Pair, 0)

	tag := kv.Pair{
		Key:   []byte("tx.proposalID"),
		Value: []byte(c.ProposalID),
	}
	tag1 := kv.Pair{
		Key:   []byte("tx.type"),
		Value: []byte(c.Type().String()),
	}
	tag2 := kv.Pair{
		Key:   []byte("tx.proposer"),
		Value: c.Proposer.Bytes(),
	}
	tag3 := kv.Pair{
		Key:   []byte("tx.proposalType"),
		Value: []byte(string(c.ProposalType)),
	}
	tag4 := kv.Pair{
		Key:   []byte("tx.initialFunding"),
		Value: []byte(c.InitialFunding.String()),
	}

	tags = append(tags, tag, tag1, tag2, tag3, tag4)
	return tags
}

func (c CreateProposal) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func (c *CreateProposal) Unmarshal(bytes []byte) error {
	return json.Unmarshal(bytes, c)
}