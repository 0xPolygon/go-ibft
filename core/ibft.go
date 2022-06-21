package core

import (
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/proto"
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
	}
}

func (i *IBFT) runRound(quit <-chan struct{}) {
	for {

		switch i.state.name {
		case new_round:
			//	TODO: id of this node
			id := []byte("my id")

			if i.backend.IsProposer(id, i.state.view.Height, i.state.view.Round) {
				proposal, err := i.backend.BuildProposal(i.state.view.Height)

				if err != nil {
					//	nesto
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
