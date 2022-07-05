package core

import (
	"github.com/Trapesys/go-ibft/messages/proto"
	"sync"
)

type stateName int

const (
	newRound stateName = iota
	prepare
	commit
	roundChange
	fin
)

// TODO make sure all fields are cleared when they should be
type state struct {
	// TODO @dbrajovic do we need this to be thread safe in the new flow?
	sync.RWMutex

	//	current view (block height, round)
	view *proto.View

	//	block proposal for current round
	proposal []byte

	//	validated commit seals
	seals [][]byte

	//	flags for different states
	roundStarted, locked bool

	name stateName
}

func (s *state) getView() *proto.View {
	s.RLock()
	defer s.RUnlock()

	return &proto.View{
		Height: s.view.Height,
		Round:  s.view.Round,
	}
}

func (s *state) getRound() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Round
}

func (s *state) isRoundStarted() bool {
	s.RLock()
	defer s.RUnlock()

	return s.roundStarted
}

func (s *state) getHeight() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Height
}

func (s *state) getProposal() []byte {
	s.RLock()
	defer s.RUnlock()

	return s.proposal
}

func (s *state) getCommittedSeals() [][]byte {
	s.RLock()
	defer s.RUnlock()

	return s.seals
}

func (s *state) isLocked() bool {
	s.RLock()
	defer s.RUnlock()

	return s.locked
}

func (s *state) getStateName() stateName {
	s.RLock()
	defer s.RUnlock()

	return s.name
}

func (s *state) setLocked(locked bool) {
	s.Lock()
	defer s.Unlock()

	s.locked = locked
}

func (s *state) setRound(round uint64) {
	s.Lock()
	defer s.Unlock()

	s.view.Round = round
}

func (s *state) setStateName(name stateName) {
	s.Lock()
	defer s.Unlock()

	s.name = name
}

func (s *state) setRoundStarted(started bool) {
	s.Lock()
	defer s.Unlock()

	s.roundStarted = started
}

func (s *state) setProposal(proposal []byte) {
	s.Lock()
	defer s.Unlock()

	s.proposal = proposal
}

func (s *state) setView(view *proto.View) {
	s.Lock()
	defer s.Unlock()

	s.view = view
}
