package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Trapesys/go-ibft/messages/proto"
)

//	New Round

func TestNewRound_Proposer(t *testing.T) {
	t.Run(
		"proposer builds proposal",
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
		})

	t.Run(
		"proposer fails to build proposal",
		func(t *testing.T) {
			//	#1:	setup prestate

			//	create ibft (proposer)

			//	not locked

			//	#2:	run cycle

			//	build proposal

			//	!ok -> round change

			//	#3:	assert
		})

}
