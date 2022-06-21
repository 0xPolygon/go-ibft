package core

import (
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/messages/proto"
)

type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

type QuorumFn func(num uint64) uint64

type view struct {
	height, round uint64
}

type stateName int

const (
	new_round stateName = iota
	prepare
	commit
	round_change
	fin
)

type state struct {
	//	current view (block height, round)
	view proto.View

	//	block proposal for current round
	proposal []byte

	//	flags for different states
	roundStarted, locked bool

	name stateName
}

type IBFT struct {
	log Logger

	state state

	messages messages.Messages

	backend Backend

	transport Transport

	quorumFn QuorumFn

	roundDone chan error
}

func NewIBFT(
	log Logger,
	backend Backend,
	transport Transport,
) *IBFT {
	return &IBFT{
		log:       log,
		backend:   backend,
		transport: transport,
		roundDone: make(chan error),
	}
}

func (i *IBFT) runRound(quit <-chan struct{}) {
	for {

		switch i.state.name {
		case new_round:
			if err := i.runNewRound(); err != nil {
				//	something wrong -> go to round change
				i.roundDone <- err
				i.state.name = round_change

				return
			}

			i.state.name = prepare
		case prepare:
		}

		//	TODO: check f+1 RC

		select {
		case <-quit:
			return
		default:
		}

	}
}

func (i *IBFT) runNewRound() error {
	var (
		height = i.state.view.Height
		round  = i.state.view.Round
		id     = []byte("my id") //	TODO: id of this node

	)

	if i.backend.IsProposer(id, height, round) {
		var (
			proposal []byte
			err      error
		)

		if i.state.locked {
			proposal = i.state.proposal
		} else {
			proposal, err = i.backend.BuildProposal(height)
			if err != nil {
				return err
			}
		}

		i.transport.Multicast(&proto.Message{
			View:      &i.state.view,
			From:      nil,
			Signature: nil,
			Type:      0,
			Payload: &proto.Message_PreprepareData{
				PreprepareData: &proto.PrePrepareMessage{
					Proposal: proposal,
				},
			},
		})

	}

	return nil
}
