package core

import (
	"bytes"
	"errors"
	"github.com/Trapesys/go-ibft/messages/proto"
)

type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

type Messages interface {
	AddMessage(message *proto.Message)
	NumMessages(view *proto.View, messageType proto.MessageType) int
	PruneByHeight(view *proto.View)
	PruneByRound(view *proto.View)
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

	messages Messages

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

func (i *IBFT) runSequence(h uint64) {
	//	TODO
}

func (i *IBFT) runRound(quit <-chan struct{}) {
	for {

		switch i.state.name {
		case new_round:
			if err := i.runNewRound(); err != nil {
				//	something wrong -> go to round change
				i.roundDone <- err

				return
			}

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
				i.state.name = round_change

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

		i.state.name = prepare

	} else {
		//	we are not the proposer, so we're checking for a PRE-PREPARE msg
		if num := i.messages.NumMessages(
			&i.state.view,
			proto.MessageType_PREPREPARE,
		); num > 0 {
			//	TODO: fetch pre-prepare message
			newProposal := []byte("new block newProposal")

			if !i.backend.IsValidBlock(newProposal) {
				i.state.name = round_change

				return errors.New("invalid block newProposal")

			}

			//	#2: I'm locked and block newProposal matches my accepted block
			if i.state.locked && !bytes.Equal(i.state.proposal, newProposal) {
				//	proposed block does not match my locked block
				i.state.name = round_change

				return errors.New("newProposal mismatch locked block")
			}

		}

		return nil
	}

	return nil
}
