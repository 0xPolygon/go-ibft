package core

import (
	"bytes"
	"errors"
	"github.com/Trapesys/go-ibft/messages"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Trapesys/go-ibft/messages/proto"
)

//	New Round

func proposalMatches(proposal []byte, message *proto.Message) bool {
	if message == nil || message.Type != proto.MessageType_PREPREPARE {
		return false
	}

	extractedProposal := message.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal

	return bytes.Equal(proposal, extractedProposal)
}

func prepareHashMatches(prepareHash []byte, message *proto.Message) bool {
	if message == nil || message.Type != proto.MessageType_PREPARE {
		return false
	}

	extractedPrepareHash := message.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash

	return bytes.Equal(prepareHash, extractedPrepareHash)
}

func commitHashMatches(commitHash []byte, message *proto.Message) bool {
	if message == nil || message.Type != proto.MessageType_COMMIT {
		return false
	}

	extractedCommitHash := message.Payload.(*proto.Message_CommitData).CommitData.ProposalHash

	return bytes.Equal(commitHash, extractedCommitHash)
}

// TestRunNewRound_Proposer checks that the node functions
// correctly as the proposer for a block
func TestRunNewRound_Proposer(t *testing.T) {
	t.Parallel()

	t.Run(
		"proposer builds block",
		func(t *testing.T) {
			t.Parallel()

			var (
				newProposal                        = []byte("new block")
				multicastedProposal *proto.Message = nil

				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {
					if message != nil && message.Type == proto.MessageType_PREPREPARE {
						multicastedProposal = message
					}
				}}
				backend = mockBackend{
					idFn: func() []byte { return nil },
					isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
						return true
					},
					buildProposalFn: func(_ uint64) ([]byte, error) {
						return newProposal, nil
					},
					buildPrePrepareMessageFn: func(proposal []byte, view *proto.View) *proto.Message {
						return &proto.Message{
							View: view,
							Type: proto.MessageType_PREPREPARE,
							Payload: &proto.Message_PreprepareData{
								PreprepareData: &proto.PrePrepareMessage{
									Proposal: proposal,
								},
							},
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)

			quitCh := make(chan struct{}, 1)
			quitCh <- struct{}{}

			i.runRound(quitCh)

			// Make sure the node is in preprepare state
			assert.Equal(t, prepare, i.state.name)

			// Make sure the accepted proposal is the one proposed to other nodes
			assert.Equal(t, newProposal, i.state.proposal)

			// Make sure the accepted proposal matches what was built
			assert.True(t, proposalMatches(newProposal, multicastedProposal))
		},
	)

	t.Run(
		"proposer fails to build proposal",
		func(t *testing.T) {
			t.Parallel()

			var (
				multicastedMessage *proto.Message = nil

				log       = mockLogger{}
				transport = mockTransport{
					multicastFn: func(message *proto.Message) {
						multicastedMessage = message
					},
				}
				backend = mockBackend{
					idFn: func() []byte { return nil },
					isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
						return true
					},
					buildProposalFn: func(_ uint64) ([]byte, error) {
						return nil, errors.New("unable to build proposal")
					},
					buildRoundChangeMessageFn: func(height uint64, round uint64) *proto.Message {
						return &proto.Message{
							View: &proto.View{
								Height: height,
								Round:  round,
							},
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)

			quitCh := make(chan struct{}, 1)
			quitCh <- struct{}{}

			i.runRound(quitCh)

			// Make sure the state is round change
			assert.Equal(t, roundChange, i.state.name)

			// Make sure the round is not started
			assert.Equal(t, false, i.state.roundStarted)

			// Make sure the round is increased
			assert.Equal(t, uint64(1), i.state.view.Round)

			// Make sure the correct message view was multicasted
			assert.Equal(t, uint64(0), multicastedMessage.View.Height)
			assert.Equal(t, uint64(1), multicastedMessage.View.Round)

			// Make sure the proposal is not accepted
			assert.Equal(t, []byte(nil), i.state.proposal)
		},
	)

	t.Run(
		"(locked) proposer builds proposal",
		func(t *testing.T) {
			t.Parallel()

			var (
				multicastedPreprepare *proto.Message = nil
				multicastedPrepare    *proto.Message = nil
				proposalHash                         = []byte("proposal hash")
				previousProposal                     = []byte("previously locked block")

				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {
					switch message.Type {
					case proto.MessageType_PREPREPARE:
						multicastedPreprepare = message
					case proto.MessageType_PREPARE:
						multicastedPrepare = message
					default:
					}
				}}
				backend = mockBackend{
					idFn: func() []byte { return nil },
					isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
						return true
					},
					buildPrepareMessageFn: func(_ []byte, view *proto.View) *proto.Message {
						return &proto.Message{
							View: view,
							Type: proto.MessageType_PREPARE,
							Payload: &proto.Message_PrepareData{
								PrepareData: &proto.PrepareMessage{
									ProposalHash: proposalHash,
								},
							},
						}
					},
					buildPrePrepareMessageFn: func(_ []byte, view *proto.View) *proto.Message {
						return &proto.Message{
							View: view,
							Type: proto.MessageType_PREPREPARE,
							Payload: &proto.Message_PreprepareData{
								PreprepareData: &proto.PrePrepareMessage{
									Proposal: previousProposal,
								},
							},
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.state.locked = true
			i.state.proposal = previousProposal

			quitCh := make(chan struct{}, 1)
			quitCh <- struct{}{}

			i.runRound(quitCh)

			// Make sure the node changed the state to prepare
			assert.Equal(t, prepare, i.state.name)

			// Make sure the locked proposal is the accepted proposal
			assert.Equal(t, previousProposal, i.state.proposal)

			// Make sure the locked proposal was multicasted
			assert.True(t, proposalMatches(previousProposal, multicastedPreprepare))

			// Make sure the correct proposal hash was multicasted
			assert.True(t, prepareHashMatches(proposalHash, multicastedPrepare))
		},
	)
}

// TestRunNewRound_Validator checks that the node functions correctly
// when receiving a proposal from another node
func TestRunNewRound_Validator(t *testing.T) {
	t.Parallel()

	t.Run(
		"validator receives valid block proposal",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				proposal                          = []byte("new block")
				proposalHash                      = []byte("proposal hash")
				multicastedPrepare *proto.Message = nil

				log       = mockLogger{}
				transport = mockTransport{
					func(message *proto.Message) {
						if message != nil && message.Type == proto.MessageType_PREPARE {
							multicastedPrepare = message

							// Close the channel as soon as the run loop parses
							// the proposal event
							quitCh <- struct{}{}
						}
					},
				}
				backend = mockBackend{
					idFn: func() []byte {
						return nil
					},
					buildPrepareMessageFn: func(proposal []byte, view *proto.View) *proto.Message {
						return &proto.Message{
							View: view,
							Type: proto.MessageType_PREPARE,
							Payload: &proto.Message_PrepareData{
								PrepareData: &proto.PrepareMessage{
									ProposalHash: proposalHash,
								},
							},
						}
					},
					isProposerFn: func(_ []byte, _, _ uint64) bool {
						return false
					},
					isValidBlockFn: func(_ []byte) bool {
						return true
					},
				}
				messages = mockMessages{
					getPrePrepareMessageFn: func(view *proto.View) *messages.PrePrepareMessage {
						return &messages.PrePrepareMessage{
							Proposal: proposal,
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.verifiedMessages = messages

			// Simulate an incoming proposal
			i.eventCh <- proposalReceived

			i.runRound(quitCh)

			// Make sure the node moves to prepare state
			assert.Equal(t, prepare, i.state.name)

			// Make sure the accepted proposal is the one that was sent out
			assert.Equal(t, proposal, i.state.proposal)

			// Make sure the correct proposal hash was multicasted
			assert.True(t, prepareHashMatches(proposalHash, multicastedPrepare))
		},
	)

	t.Run(
		"validator receives invalid block proposal",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				multicastedRoundChange *proto.Message = nil

				log       = mockLogger{}
				transport = mockTransport{
					multicastFn: func(message *proto.Message) {
						if message != nil && message.Type == proto.MessageType_ROUND_CHANGE {
							multicastedRoundChange = message

							// Close the channel as soon as the run loop parses
							// the proposal event
							quitCh <- struct{}{}
						}
					},
				}
				backend = mockBackend{
					idFn: func() []byte {
						return nil
					},
					isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
						return false
					},
					isValidBlockFn: func(_ []byte) bool {
						return false
					},
					buildRoundChangeMessageFn: func(height uint64, round uint64) *proto.Message {
						return &proto.Message{
							View: &proto.View{
								Height: height,
								Round:  round,
							},
							Type: proto.MessageType_ROUND_CHANGE,
						}
					},
				}
			)

			i := NewIBFT(log, backend, transport)

			// Simulate an incoming proposal
			i.errorCh <- errInvalidBlockProposal

			i.runRound(quitCh)

			// Make sure the node moved to round change state
			assert.Equal(t, roundChange, i.state.name)

			// Make sure the round is not started
			assert.Equal(t, false, i.state.roundStarted)

			// Make sure the round is increased
			assert.Equal(t, uint64(1), i.state.view.Round)

			// Make sure the correct message view was multicasted
			assert.Equal(t, uint64(0), multicastedRoundChange.View.Height)
			assert.Equal(t, uint64(1), multicastedRoundChange.View.Round)

			// Make sure the proposal is not accepted
			assert.Equal(t, []byte(nil), i.state.proposal)
		},
	)
}

// TestRunPrepare checks that the node behaves correctly
// in prepare state
func TestRunPrepare(t *testing.T) {
	t.Parallel()

	t.Run(
		"validator receives quorum of PREPARE messages",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				quorum   = uint64(4)
				quorumFn = QuorumFn(
					func(_ uint64) uint64 {
						return quorum
					},
				)
				proposal                         = []byte("block proposal")
				proposalHash                     = []byte("proposal hash")
				multicastedCommit *proto.Message = nil
				capturedState                    = newRound

				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {
					if message != nil && message.Type == proto.MessageType_COMMIT {
						multicastedCommit = message

						// Make sure the run loop closes as soon as it's done
						// parsing the prepares received event
						quitCh <- struct{}{}
					}
				}}
				backend = mockBackend{
					buildCommitMessageFn: func(bytes []byte, view *proto.View) *proto.Message {
						return &proto.Message{
							View: view,
							Type: proto.MessageType_COMMIT,
							Payload: &proto.Message_CommitData{
								CommitData: &proto.CommitMessage{
									ProposalHash: proposalHash,
								},
							},
						}
					},
				}
			)
			i := NewIBFT(log, backend, transport)
			i.quorumFn = quorumFn
			i.state.proposal = proposal

			i.eventCh <- quorumPrepares

			// Make sure the proper event is emitted
			var wg sync.WaitGroup
			wg.Add(1)
			go func(i *IBFT) {
				defer wg.Done()

				select {
				case name := <-i.stateChangeCh:
					capturedState = name
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			i.runRound(quitCh)

			// Make sure the node moves to the commit state
			assert.Equal(t, commit, i.state.name)

			// Make sure the node stays locked on a block
			assert.True(t, i.state.locked)

			// Make sure the proposal didn't change
			assert.Equal(t, proposal, i.state.proposal)

			// Make sure the proper proposal hash was multicasted
			assert.True(t, commitHashMatches(proposalHash, multicastedCommit))

			// Make sure the proper event was emitted
			wg.Wait()
			assert.Equal(t, commit, capturedState)
		},
	)
}

/*

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

				assert.ErrorIs(t, errQuorumNotReached, i.runCommit())
				assert.NotEqual(t, fin, i.state.name)
			})
*/

/*

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
*/
