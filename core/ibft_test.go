package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Trapesys/go-ibft/messages/proto"
)

//	New Round

func TestRunNewRound_Proposer(t *testing.T) {
	t.Run(
		"proposer builds proposal",
		func(t *testing.T) {
			var (
				newProposal = []byte("new block")

				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {}}
				backend   = mockBackend{
					isProposerFn:    func(bytes []byte, u uint64, u2 uint64) bool { return true },
					buildProposalFn: func(u uint64) ([]byte, error) { return newProposal, nil },
				}
			)

			i := NewIBFT(log, backend, transport)

			assert.NoError(t, i.runNewRound())
			assert.Equal(t, prepare, i.state.name)
			assert.Equal(t, newProposal, i.state.proposal)
		},
	)

	t.Run(
		"proposer fails to build proposal",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return true
					},
					buildProposalFn: func(u uint64) ([]byte, error) {
						return nil, errors.New("bad")
					},
				}
			)

			i := NewIBFT(log, backend, transport)

			assert.ErrorIs(t, errBuildProposal, i.runNewRound())
			assert.Equal(t, roundChange, i.state.name)
			assert.Equal(t, []byte(nil), i.state.proposal)
		},
	)

	t.Run(
		"(locked) proposer builds proposal",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {}}
				backend   = mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return true
					},
				}
			)

			previousProposal := []byte("previously locked block")

			i := NewIBFT(log, backend, transport)
			i.state.locked = true
			i.state.proposal = previousProposal

			assert.NoError(t, i.runNewRound())
			assert.Equal(t, prepare, i.state.name)
			assert.Equal(t, previousProposal, i.state.proposal)
		},
	)
}

func TestNewRound_Validator(t *testing.T) {
	t.Run(
		"invalid PRE-PREPARE received",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return false
					},
				},
				&mockTransport{},
			)

			i.messages = mockMessages{
				numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
					return 1
				},
			}

			i.state.name = newRound
			i.state.locked = false

			go func() {
				//	unblock runRound
				<-i.roundDone
			}()

			i.runRound(nil)

			assert.Equal(t, roundChange, i.state.name)
		},
	)

	t.Run(
		"PRE-PREPARE does not match locked block",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return true
					},
				},
				&mockTransport{},
			)

			i.messages = mockMessages{
				numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
					return 1
				},
			}

			i.state.name = newRound
			i.state.locked = true
			i.state.proposal = []byte("old block")

			go func() {
				//	unblock runRound
				<-i.roundDone
			}()

			i.runRound(nil)

			assert.Equal(t, roundChange, i.state.name)
		},
	)

	t.Run(
		"valid PRE-PREPARE received",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return true
					},
				},
				&mockTransport{
					func(message *proto.Message) {},
				},
			)

			i.messages = mockMessages{
				numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
					return 1
				},
			}

			i.state.name = newRound
			i.state.locked = false

			quit := make(chan struct{})
			go func() {
				close(quit)
			}()

			i.runRound(quit)

			assert.Equal(t, prepare, i.state.name)
		},
	)

	t.Run(
		"PRE-PREPARE matches locked block",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return true
					},
				},
				&mockTransport{
					func(message *proto.Message) {},
				},
			)

			i.messages = mockMessages{
				numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
					return 1
				},
			}

			i.state.name = newRound
			i.state.locked = true
			i.state.proposal = []byte("new block")

			quit := make(chan struct{})
			go func() {
				close(quit)
			}()

			i.runRound(quit)

			assert.Equal(t, prepare, i.state.name)
		},
	)

}
