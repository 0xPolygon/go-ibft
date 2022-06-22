package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Trapesys/go-ibft/messages/proto"
)

//	New Round

func TestNewRound_Proposer(t *testing.T) {
	t.Run(
		"unlocked proposer builds proposal",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&MockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return true
					},
				},
				&mockTransport{func(message *proto.Message) {}},
			)

			i.state.locked = false

			//	close the channel so runRound completes
			quit := make(chan struct{})
			go func() {
				close(quit)
			}()

			i.runRound(quit)

			assert.Equal(t, prepare, i.state.name)
		},
	)

	t.Run(
		"unlocked proposer fails to build proposal",
		func(t *testing.T) {
			//	#1:	setup prestate
			i := NewIBFT(
				&mockLogger{},
				&MockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return true
					},
					buildProposalFn: func(u uint64) ([]byte, error) {
						return nil, errors.New("bad")
					},
				},
				&mockTransport{func(message *proto.Message) {}},
			)

			i.state.locked = false

			//	unblock runRound
			go func() {
				<-i.roundDone
			}()

			i.runRound(nil)

			assert.Equal(t, round_change, i.state.name)
		},
	)

	t.Run(
		"locked proposer builds proposal",
		func(t *testing.T) {
			//	#1:	setup prestate
			i := NewIBFT(
				&mockLogger{},
				&MockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return true
					},
				},
				&mockTransport{func(message *proto.Message) {}},
			)

			i.state.locked = true
			i.state.proposal = []byte("previously locked block")

			//	close the channel so runRound completes
			quit := make(chan struct{})
			go func() {
				close(quit)
			}()

			i.runRound(quit)

			assert.Equal(t, prepare, i.state.name)
		},
	)
}

func TestNewRound_Validator(t *testing.T) {
	t.Run(
		"invalid PRE-PREPARE received",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&MockBackend{
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

			i.state.name = new_round
			i.state.locked = false

			go func() {
				//	unblock runRound
				<-i.roundDone
			}()

			i.runRound(nil)

			assert.Equal(t, round_change, i.state.name)
		},
	)

	t.Run(
		"PRE-PREPARE does not match locked block",
		func(t *testing.T) {
			i := NewIBFT(
				&mockLogger{},
				&MockBackend{
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

			i.state.name = new_round
			i.state.locked = true
			i.state.proposal = []byte("old block")

			go func() {
				//	unblock runRound
				<-i.roundDone
			}()

			i.runRound(nil)

			assert.Equal(t, round_change, i.state.name)
		},
	)

	t.Run(
		"valid PRE-PREPARE received",
		func(t *testing.T) {

		},
	)

	t.Run(
		"PRE-PREPARE matches locked block",
		func(t *testing.T) {

		},
	)

}
