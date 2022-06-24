package core

import (
	"errors"
	"github.com/Trapesys/go-ibft/messages"
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

func TestRunNewRound_Validator(t *testing.T) {
	t.Run(
		"validator receives valid block proposal",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {}}
				backend   = mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return true
					},
				}
				messages = mockMessages{
					numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
						return 1
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages

			assert.NoError(t, i.runNewRound())
			assert.Equal(t, prepare, i.state.name)
			assert.Equal(t, []byte("new block"), i.state.proposal) // TODO: fix
		},
	)

	t.Run(
		"validator receives invalid block proposal",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return false
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = mockMessages{
				numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
					return 1
				},
			}

			assert.ErrorIs(t, errInvalidBlock, i.runNewRound())
			assert.Equal(t, roundChange, i.state.name)
		},
	)

	t.Run(
		"(locked) validator receives mismatching proposal",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return true
					},
				}
				messages = mockMessages{
					numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
						return 1
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages

			i.state.locked = true
			i.state.proposal = []byte("old block")

			assert.ErrorIs(t, errPrePrepareBlockMismatch, i.runNewRound())
			assert.Equal(t, roundChange, i.state.name)
			assert.Equal(t, []byte("old block"), i.state.proposal)
		},
	)

	t.Run(
		"(locked) validator receives matching proposal",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					isProposerFn: func(bytes []byte, u uint64, u2 uint64) bool {
						return false
					},
					isValidBlockFn: func(bytes []byte) bool {
						return true
					},
				}
				messages = mockMessages{
					numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
						return 1
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.state.locked = true
			i.state.proposal = []byte("new block")

			assert.NoError(t, i.runNewRound())
			assert.Equal(t, prepare, i.state.name)
			assert.Equal(t, []byte("new block"), i.state.proposal)
		},
	)
}

func TestRunPrepare(t *testing.T) {
	t.Run(
		"validator receives quorum of PREPARE messages",
		func(t *testing.T) {
			var (
				quorum   = uint64(4)
				quorumFn = QuorumFn(func(num uint64) uint64 { return quorum })

				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {}}
				backend   = mockBackend{
					buildCommitMessageFn: func(bytes []byte) *proto.Message { return &proto.Message{} },
					verifyProposalHashFn: func(bytes []byte, bytes2 []byte) error { return nil },
					validatorCountFn: func(blockNumber uint64) uint64 {
						return 4
					},
				}
				messages = mockMessages{
					getPrepareMessagesFn: func(view *proto.View) []*messages.PrepareMessage {
						return []*messages.PrepareMessage{
							0: {ProposalHash: []byte("hash")},
							1: {ProposalHash: []byte("hash")},
							2: {ProposalHash: []byte("hash")},
							3: {ProposalHash: []byte("hash")},
						}
					},
					numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
						return 4
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.quorumFn = quorumFn

			assert.NoError(t, i.runPrepare())
			assert.Equal(t, commit, i.state.name)
			assert.True(t, i.state.locked)
		},
	)

	t.Run(
		"validator not (yet) received quorum",
		func(t *testing.T) {
			var (
				quorum   = uint64(4)
				quorumFn = QuorumFn(func(num uint64) uint64 { return quorum })

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					validatorCountFn: func(blockNumber uint64) uint64 {
						return 4
					},
				}
				messages = mockMessages{
					getPrepareMessagesFn: func(view *proto.View) []*messages.PrepareMessage {
						return []*messages.PrepareMessage{
							0: {ProposalHash: []byte("hash")},
						}
					},
					numMessagesFn: func(view *proto.View, messageType proto.MessageType) int {
						return 4
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.quorumFn = quorumFn
			i.state.name = prepare

			//println(i.runPrepare())
			assert.ErrorIs(t, errQuorumNotReached, i.runPrepare())
			assert.Equal(t, prepare, i.state.name)
			assert.False(t, i.state.locked)
		},
	)
}

func TestRunCommit(t *testing.T) {
	t.Run(
		"validator received quorum of valid commit messages",
		func(t *testing.T) {
			var (
				quorum   = 2
				quorumFn = QuorumFn(func(num uint64) uint64 { return uint64(quorum) })

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					validatorCountFn: func(blockNumber uint64) uint64 {
						return 4
					},
					isValidCommittedSealFn: func(bytes []byte, bytes2 []byte) bool {
						return true
					},
				}
				messages = mockMessages{
					getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
						return []*messages.CommitMessage{
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.quorumFn = quorumFn

			assert.NoError(t, i.runCommit())
			assert.Equal(t, fin, i.state.name)
		},
	)

	t.Run(
		"validator not reaching quorum of commit messages",
		func(t *testing.T) {
			var (
				quorum   = 4
				quorumFn = QuorumFn(func(num uint64) uint64 { return uint64(quorum) })

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					validatorCountFn: func(blockNumber uint64) uint64 {
						return 4
					},
				}
				messages = mockMessages{
					getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
						return []*messages.CommitMessage{
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.quorumFn = quorumFn

			assert.ErrorIs(t, errQuorumNotReached, i.runCommit())
			assert.NotEqual(t, fin, i.state.name)
		},
	)

	t.Run(
		"validator received commit messages with invalid seals",
		func(t *testing.T) {
			var (
				quorum   = 2
				quorumFn = QuorumFn(func(num uint64) uint64 { return uint64(quorum) })

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					validatorCountFn: func(blockNumber uint64) uint64 {
						return 4
					},
					isValidCommittedSealFn: func(bytes []byte, bytes2 []byte) bool {
						return false
					},
				}
				messages = mockMessages{
					getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
						return []*messages.CommitMessage{
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("invalid seal")},
							{ProposalHash: []byte("hash"), CommittedSeal: []byte("seal")},
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.quorumFn = quorumFn

			assert.ErrorIs(t, errInvalidCommittedSeal, i.runCommit())
			assert.NotEqual(t, fin, i.state.name)
		})

}

func TestRunFin(t *testing.T) {
	t.Run(
		"validator insert finalized block",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					insertBlockFn: func(bytes []byte, i [][]byte) error {
						return nil
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			assert.NoError(t, i.runFin())
		},
	)

	t.Run(
		"validator insert finalized block",
		func(t *testing.T) {
			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					insertBlockFn: func(bytes []byte, i [][]byte) error {
						return errors.New("bad")
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			assert.ErrorIs(t, errInsertBlock, i.runFin())
		},
	)
}
