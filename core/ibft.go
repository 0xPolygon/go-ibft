package core

import (
	"github.com/Trapesys/go-ibft/messages"
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

type state struct {
	//	current view (block height, round)
	view view

	//	block proposal for current round
	proposal []byte

	//	flags for different states
	roundStarted, locked bool
}

type IBFT struct {
	log Logger

	state state

	messages messages.Messages

	backend Backend

	transport Transport

	quorumFn QuorumFn
}
