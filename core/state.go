package core

import (
	"github.com/Trapesys/go-ibft/messages/proto"
	"sync"
)

type stateType uint8

const (
	newRound stateType = iota
	prepare
	commit
	roundChange
	fin
)

func (s stateType) String() (str string) {
	switch s {
	case newRound:
		str = "new round"
	case prepare:
		str = "prepare"
	case commit:
		str = "commit"
	case roundChange:
		str = "round change"
	case fin:
		str = "fin"
	}

	return
}

type state struct {
	sync.RWMutex

	//	current view (block height, round)
	view *proto.View

	//	block proposal for current round
	proposal []byte

	//	validated commit seals
	seals [][]byte

	//	flags for different states
	roundStarted, locked bool

	name stateType
}

func (s *state) getView() *proto.View {
	s.RLock()
	defer s.RUnlock()

	return &proto.View{
		Height: s.view.Height,
		Round:  s.view.Round,
	}
}

func (s *state) clear(height uint64) {
	s.Lock()
	defer s.Unlock()

	s.proposal = nil
	s.seals = nil
	s.roundStarted = false
	s.locked = false
	s.name = newRound

	s.view = &proto.View{
		Height: height,
		Round:  0,
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

func (s *state) getStateName() stateType {
	s.RLock()
	defer s.RUnlock()

	return s.name
}

func (s *state) setLocked(locked bool) {
	s.Lock()
	defer s.Unlock()

	s.locked = locked
}

func (s *state) setStateName(name stateType) {
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

func (s *state) setCommittedSeals(seals [][]byte) {
	s.Lock()
	defer s.Unlock()

	s.seals = seals
}
