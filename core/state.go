package core

import (
	"math/big"
	"sync"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
)

type stateType uint8

const (
	newRound stateType = iota
	prepare
	commit
	fin
)

func (s stateType) String() string {
	switch s {
	case newRound:
		return "new round"
	case prepare:
		return "prepare"
	case commit:
		return "commit"
	case fin:
		return "fin"
	}

	return ""
}

type state struct {
	sync.RWMutex

	//	current view (sequence, round)
	view *proto.View

	// latestPC is the latest prepared certificate
	latestPC *proto.PreparedCertificate

	// latestPreparedProposal is the proposal
	// for which Q(N)-1 PREPARE messages were received
	latestPreparedProposal *proto.Proposal

	//	accepted proposal for current round
	proposalMessage *proto.Message

	//	validated commit seals
	seals []*messages.CommittedSeal

	//	flags for different states
	roundStarted bool

	name stateType

	// quorumSize represents quorum for the height specified in the current View
	quorumSize *big.Int

	// validatorsVotingPower is a map of the validator addresses on their voting power for
	// the height specified in the current View
	//TODO add address instead of string
	validatorsVotingPower map[string]*big.Int
}

func (s *state) getView() *proto.View {
	s.RLock()
	defer s.RUnlock()

	return &proto.View{
		Height: s.view.Height,
		Round:  s.view.Round,
	}
}

func (s *state) reset(i *IBFT, height uint64) error {
	s.Lock()
	defer s.Unlock()

	s.seals = nil
	s.roundStarted = false
	s.name = newRound
	s.proposalMessage = nil
	s.latestPC = nil
	s.latestPreparedProposal = nil

	s.view = &proto.View{
		Height: height,
		Round:  0,
	}

	return s.setQuorumData(i)
}

func (s *state) getLatestPC() *proto.PreparedCertificate {
	s.RLock()
	defer s.RUnlock()

	return s.latestPC
}

func (s *state) getLatestPreparedProposal() *proto.Proposal {
	s.RLock()
	defer s.RUnlock()

	return s.latestPreparedProposal
}

func (s *state) getProposalMessage() *proto.Message {
	s.RLock()
	defer s.RUnlock()

	return s.proposalMessage
}

func (s *state) getProposalHash() []byte {
	s.RLock()
	defer s.RUnlock()

	return messages.ExtractProposalHash(s.proposalMessage)
}

func (s *state) setProposalMessage(proposalMessage *proto.Message) {
	s.Lock()
	defer s.Unlock()

	s.proposalMessage = proposalMessage
}

func (s *state) getRound() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Round
}

func (s *state) getHeight() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Height
}

func (s *state) getProposal() *proto.Proposal {
	s.RLock()
	defer s.RUnlock()

	if s.proposalMessage != nil {
		return messages.ExtractProposal(s.proposalMessage)
	}

	return nil
}

func (s *state) getRawDataFromProposal() []byte {
	proposal := s.getProposal()

	if proposal != nil {
		return proposal.RawProposal
	}

	return nil
}

func (s *state) getCommittedSeals() []*messages.CommittedSeal {
	s.RLock()
	defer s.RUnlock()

	return s.seals
}

func (s *state) getStateName() stateType {
	s.RLock()
	defer s.RUnlock()

	return s.name
}

func (s *state) changeState(name stateType) {
	s.Lock()
	defer s.Unlock()

	s.name = name
}

func (s *state) setRoundStarted(started bool) {
	s.Lock()
	defer s.Unlock()

	s.roundStarted = started
}

func (s *state) setView(view *proto.View) {
	s.Lock()
	defer s.Unlock()

	s.view = view
}

func (s *state) setCommittedSeals(seals []*messages.CommittedSeal) {
	s.Lock()
	defer s.Unlock()

	s.seals = seals
}

func (s *state) newRound() {
	s.Lock()
	defer s.Unlock()

	if !s.roundStarted {
		// Round is not yet started, kick the round off
		s.name = newRound
		s.roundStarted = true
	}
}

func (s *state) finalizePrepare(
	certificate *proto.PreparedCertificate,
	latestPPB *proto.Proposal,
) {
	s.Lock()
	defer s.Unlock()

	s.latestPC = certificate
	s.latestPreparedProposal = latestPPB

	// Move to the commit state
	s.name = commit
}

func (s *state) setQuorumData(i *IBFT) error {
	var err error
	if s.validatorsVotingPower, err = i.backend.GetVotingPower(s.view.Height); err != nil {
		return err
	}

	totalVotingPower := calculateTotalVotingPower(s.validatorsVotingPower)
	s.quorumSize = calculateQuorum(totalVotingPower)

	return nil
}

func calculateQuorum(totalVotingPower *big.Int) *big.Int {
	quorum := new(big.Int)
	quorum.Mul(totalVotingPower, big.NewInt(2))

	return BigIntDivCeil(quorum, big.NewInt(3))
}

func calculateTotalVotingPower(validatorsVotingPower map[string]*big.Int) *big.Int {
	totalVotingPower := big.NewInt(0)
	for _, validatorVotingPower := range validatorsVotingPower {
		totalVotingPower = totalVotingPower.Add(totalVotingPower, validatorVotingPower)
	}

	return totalVotingPower
}

// BigIntDivCeil performs integer division and rounds given result to next bigger integer number
// It is calculated using this formula result = (a + b - 1) / b
func BigIntDivCeil(a, b *big.Int) *big.Int {
	result := new(big.Int)

	return result.Add(a, b).
		Sub(result, big.NewInt(1)).
		Div(result, b)
}
