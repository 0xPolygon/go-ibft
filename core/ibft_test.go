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
					buildCommitMessageFn: func(_ []byte, view *proto.View) *proto.Message {
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

// TestRunCommit makes sure the node
// behaves correctly in the commit state
func TestRunCommit(t *testing.T) {
	t.Parallel()

	t.Run(
		"validator received quorum of valid commit messages",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				proposal                        = []byte("block proposal")
				insertedProposal       []byte   = nil
				insertedCommittedSeals [][]byte = nil
				capturedEvent                   = noEvent
				committedSeals                  = [][]byte{[]byte("seal"), []byte("seal")}

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					insertBlockFn: func(proposal []byte, committedSeals [][]byte) error {
						insertedProposal = proposal
						insertedCommittedSeals = committedSeals

						return nil
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.verifiedMessages = mockMessages{}
			i.unverifiedMessages = mockMessages{}
			i.state.proposal = proposal
			i.state.seals = committedSeals

			// Make sure the proper event is emitted
			var wg sync.WaitGroup
			wg.Add(1)
			go func(i *IBFT) {
				defer func() {
					wg.Done()

					// Close out the main run loop
					// as soon as the quorum commit event is parsed
					quitCh <- struct{}{}
				}()

				select {
				case event := <-i.roundDone:
					capturedEvent = event
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			i.eventCh <- quorumCommits

			i.runRound(quitCh)

			// Make sure the node changed the state to fin
			assert.Equal(t, fin, i.state.name)

			// Make sure the inserted proposal was the one present
			assert.Equal(t, insertedProposal, proposal)

			// Make sure the inserted committed seals were correct
			assert.Equal(t, insertedCommittedSeals, committedSeals)

			// Make sure the proper event was emitted
			wg.Wait()
			assert.Equal(t, consensusReached, capturedEvent)
		},
	)

	t.Run(
		"validator failed to insert the finalized block",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				multicastedRoundChange *proto.Message = nil

				log       = mockLogger{}
				transport = mockTransport{
					func(message *proto.Message) {
						if message != nil && message.Type == proto.MessageType_ROUND_CHANGE {
							multicastedRoundChange = message

							// Make sure to close the main run loop
							// once the round change event has been emitted
							quitCh <- struct{}{}
						}
					},
				}
				backend = mockBackend{
					insertBlockFn: func(proposal []byte, committedSeals [][]byte) error {
						return errors.New("unable to insert block")
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

			i.eventCh <- quorumCommits

			i.runRound(quitCh)

			// Make sure the node changed the state to fin
			assert.Equal(t, roundChange, i.state.name)

			// Make sure the node is not locked on the proposal
			assert.Equal(t, false, i.state.locked)

			// Make sure the round is not started
			assert.Equal(t, false, i.state.roundStarted)

			// Make sure the round is increased
			assert.Equal(t, uint64(1), i.state.view.Round)

			// Make sure the correct message view was multicasted
			assert.Equal(t, uint64(0), multicastedRoundChange.View.Height)
			assert.Equal(t, uint64(1), multicastedRoundChange.View.Round)
		},
	)
}

// TestIBFT_IsAcceptableMessage makes sure invalid messages
// are properly handled
func TestIBFT_IsAcceptableMessage(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name          string
		view          *proto.View
		invalidSender bool
		acceptable    bool
	}{
		{
			"invalid sender",
			nil,
			true,
			false,
		},
		{
			"malformed message",
			nil,
			false,
			false,
		},
		{
			"height mismatch",
			&proto.View{
				Height: 100,
			},
			false,
			false,
		},
		{
			"higher round number",
			&proto.View{
				Height: 0,
				Round:  1,
			},
			false,
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					isValidMessageFn: func(message *proto.Message) bool {
						return !testCase.invalidSender
					},
				}
			)
			i := NewIBFT(log, backend, transport)

			message := &proto.Message{
				View: testCase.view,
			}

			assert.Equal(t, testCase.acceptable, i.isAcceptableMessage(message))
		})
	}
}

// TestIBFT_CanVerifyMessage checks if certain message
// types can be verified based on the current state
func TestIBFT_CanVerifyMessage(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name         string
		messageType  proto.MessageType
		currentState stateName
		verifiable   bool
	}{
		{
			"verifiable round change message",
			proto.MessageType_ROUND_CHANGE,
			newRound,
			true,
		},
		{
			"unverifiable commit message",
			proto.MessageType_COMMIT,
			newRound,
			false,
		},
		{
			"unverifiable prepare message",
			proto.MessageType_PREPARE,
			newRound,
			false,
		},
		{
			"verifiable preprepare message",
			proto.MessageType_PREPREPARE,
			newRound,
			true,
		},
		{
			"verifiable commit message",
			proto.MessageType_COMMIT,
			prepare,
			true,
		},
		{
			"verifiable prepare message",
			proto.MessageType_PREPARE,
			prepare,
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{}
			)
			i := NewIBFT(log, backend, transport)
			i.state.name = testCase.currentState

			message := &proto.Message{
				Type: testCase.messageType,
			}

			assert.Equal(t, testCase.verifiable, i.canVerifyMessage(message))
		})
	}
}
