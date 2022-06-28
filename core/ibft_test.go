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

// TestRunRoundChange makes sure the node
// behaves correctly in the round change state
func TestRunRoundChange(t *testing.T) {
	t.Parallel()

	t.Run(
		"validator received quorum round change messages",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				capturedEvent = noEvent

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{}
				messages  = mockMessages{
					getRoundChangeMessagesFn: func(view *proto.View) []*messages.RoundChangeMessage {
						return []*messages.RoundChangeMessage{{
							Round: 1,
						}}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.verifiedMessages = messages

			// Make sure the proper event is emitted
			var wg sync.WaitGroup
			wg.Add(1)
			go func(i *IBFT) {
				defer func() {
					wg.Done()

					// Close out the main run loop
					// as soon as the quorum round change event is parsed
					quitCh <- struct{}{}
				}()

				select {
				case event := <-i.roundDone:
					capturedEvent = event
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			i.eventCh <- quorumRoundChanges

			i.runRound(quitCh)

			// Make sure the round is increased
			assert.Equal(t, uint64(1), i.state.view.Round)

			// Make sure the proper event was emitted
			wg.Wait()
			assert.Equal(t, repeatSequence, capturedEvent)
		},
	)

	t.Run(
		"validator received F+1 round change messages",
		func(t *testing.T) {
			t.Parallel()

			quitCh := make(chan struct{}, 1)

			var (
				multicastedRoundChange *proto.Message = nil
				higherRound            uint64         = 5

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
				messages = mockMessages{
					getMostRoundChangeMessagesFn: func(round uint64, height uint64) []*messages.RoundChangeMessage {
						return []*messages.RoundChangeMessage{{
							Round:  higherRound, // Higher round hop
							Height: height,
						}}
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.verifiedMessages = messages

			i.eventCh <- roundHop

			i.runRound(quitCh)

			// Make sure the node changed the state to fin
			assert.Equal(t, roundChange, i.state.name)

			// Make sure the round is not started
			assert.Equal(t, false, i.state.roundStarted)

			// Make sure the round is increased
			assert.Equal(t, higherRound, i.state.view.Round)

			// Make sure the correct message view was multicasted
			assert.Equal(t, uint64(0), multicastedRoundChange.View.Height)
			assert.Equal(t, higherRound, multicastedRoundChange.View.Round)
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

// TestIBFT_EventPossible makes sure certain
// event emittance is possible once conditions are met
func TestIBFT_EventPossible(t *testing.T) {
	t.Parallel()

	generatePrepareMessages := func(count uint64) []*messages.PrepareMessage {
		prepares := make([]*messages.PrepareMessage, count)

		for index := uint64(0); index < count; index++ {
			prepares[index] = &messages.PrepareMessage{ProposalHash: nil}
		}

		return prepares
	}

	generateCommitMessages := func(count uint64) []*messages.CommitMessage {
		commits := make([]*messages.CommitMessage, count)

		for index := uint64(0); index < count; index++ {
			commits[index] = &messages.CommitMessage{ProposalHash: nil}
		}

		return commits
	}

	generateRoundChangeMessages := func(count uint64) []*messages.RoundChangeMessage {
		roundChanges := make([]*messages.RoundChangeMessage, count)

		for index := uint64(0); index < count; index++ {
			roundChanges[index] = &messages.RoundChangeMessage{}
		}

		return roundChanges
	}

	testTable := []struct {
		name          string
		messageType   proto.MessageType
		expectedEvent event
		quorum        uint64
		messages      mockMessages
	}{
		{
			"preprepare received",
			proto.MessageType_PREPREPARE,
			proposalReceived,
			1,
			mockMessages{},
		},
		{
			"quorum prepares received",
			proto.MessageType_PREPARE,
			quorumPrepares,
			1,
			mockMessages{
				getPrepareMessagesFn: func(view *proto.View) []*messages.PrepareMessage {
					return generatePrepareMessages(1)
				},
			},
		},
		{
			"quorum (prepares + commits) received",
			proto.MessageType_PREPARE,
			quorumPrepares,
			4,
			mockMessages{
				getPrepareMessagesFn: func(view *proto.View) []*messages.PrepareMessage {
					return generatePrepareMessages(4 / 2)
				},
				getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
					return generateCommitMessages(4 / 2)
				},
			},
		},
		{
			"quorum commits received",
			proto.MessageType_COMMIT,
			quorumCommits,
			1,
			mockMessages{
				getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
					return generateCommitMessages(1)
				},
			},
		},
		{
			"quorum (commits + prepares) received",
			proto.MessageType_COMMIT,
			quorumPrepares,
			4,
			mockMessages{
				getPrepareMessagesFn: func(view *proto.View) []*messages.PrepareMessage {
					return generatePrepareMessages(4 / 2)
				},
				getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
					return generateCommitMessages(4 / 2)
				},
			},
		},
		{
			"quorum round changes received",
			proto.MessageType_ROUND_CHANGE,
			quorumRoundChanges,
			1,
			mockMessages{
				getRoundChangeMessagesFn: func(view *proto.View) []*messages.RoundChangeMessage {
					return generateRoundChangeMessages(1)
				},
			},
		},
		{
			"F+1 round changes received",
			proto.MessageType_ROUND_CHANGE,
			roundHop,
			4,
			mockMessages{
				getRoundChangeMessagesFn: func(view *proto.View) []*messages.RoundChangeMessage {
					return generateRoundChangeMessages(4 - 1)
				},
				getMostRoundChangeMessagesFn: func(u uint64, u2 uint64) []*messages.RoundChangeMessage {
					return generateRoundChangeMessages(uint64(1)) // Faulty is 0 for the mock
				},
			},
		},
		{
			"no event possible",
			proto.MessageType_PREPARE,
			noEvent,
			4,
			mockMessages{
				getPrepareMessagesFn: func(view *proto.View) []*messages.PrepareMessage {
					return generatePrepareMessages(4/2 - 1)
				},
				getCommitMessagesFn: func(view *proto.View) []*messages.CommitMessage {
					return generateCommitMessages(4/2 - 1)
				},
			},
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

			i.quorumFn = func(num uint64) uint64 {
				return testCase.quorum
			}

			i.state.view = &proto.View{
				Height: 0,
				Round:  0,
			}

			i.verifiedMessages = testCase.messages

			assert.Equal(t, testCase.expectedEvent, i.eventPossible(testCase.messageType))
		})
	}
}

func TestIBFT_ValidateMessage_Preprepare(t *testing.T) {
	t.Parallel()

	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	testTable := []struct {
		name         string
		expectedErr  error
		backend      Backend
		currentState *state
		message      *proto.Message
	}{
		{
			"preprepare view mismatch",
			errViewMismatch,
			mockBackend{},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_PREPREPARE,
				// Set a differing view
				View: &proto.View{
					Height: 0,
					Round:  1,
				},
			},
		},
		{
			"invalid proposer",
			errInvalidProposer,
			mockBackend{
				isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
					// Make sure the proposer is invalid
					return false
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_PREPREPARE,
				View: baseView,
			},
		},
		{
			"invalid proposal (locked)",
			errInvalidBlockProposed,
			mockBackend{
				isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
					return true
				},
			},
			&state{
				view:     baseView,
				locked:   true, // Make sure the proposal is locked
				proposal: []byte("block proposal"),
			},
			&proto.Message{
				View: baseView,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						// Make sure the proposal differs from the locked proposal
						Proposal: []byte("different proposal"),
					},
				},
			},
		},
		{
			"invalid proposal (validation)",
			errInvalidBlockProposal,
			mockBackend{
				isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
					return true
				},
				isValidBlockFn: func(_ []byte) bool {
					// Make sure the proposal doesn't pass validation
					return false
				},
			},
			&state{
				view:     baseView,
				locked:   true,
				proposal: []byte("block proposal"),
			},
			&proto.Message{
				View: baseView,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal: []byte("block proposal"),
					},
				},
			},
		},
		{
			"valid proposal",
			nil,
			mockBackend{
				isProposerFn: func(_ []byte, _ uint64, _ uint64) bool {
					return true
				},
				isValidBlockFn: func(_ []byte) bool {
					return true
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				View: baseView,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal: []byte("block proposal"),
					},
				},
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				log       = mockLogger{}
				transport = mockTransport{}
			)

			i := NewIBFT(log, testCase.backend, transport)
			i.state = testCase.currentState

			// Make sure the message is processed correctly
			assert.ErrorIs(t, i.validateMessage(testCase.message), testCase.expectedErr)
		})
	}
}

func TestIBFT_ValidateMessage_Prepare(t *testing.T) {
	t.Parallel()

	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	testTable := []struct {
		name         string
		expectedErr  error
		backend      Backend
		currentState *state
		message      *proto.Message
	}{
		{
			"prepare view mismatch",
			errViewMismatch,
			mockBackend{},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_PREPARE,
				// Make sure the view is different
				View: &proto.View{
					Height: 0,
					Round:  1,
				},
			},
		},
		{
			"proposal hash mismatch",
			errHashMismatch,
			mockBackend{
				verifyProposalHashFn: func(_ []byte, _ []byte) error {
					// Make sure the proposal hash is rejected
					return errors.New("invalid hash")
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_PREPARE,
				View: baseView,
				Payload: &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{
						ProposalHash: []byte("proposal hash"),
					},
				},
			},
		},
		{
			"valid prepare",
			nil,
			mockBackend{
				verifyProposalHashFn: func(_ []byte, _ []byte) error {
					return nil
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_PREPARE,
				View: baseView,
				Payload: &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{
						ProposalHash: []byte("proposal hash"),
					},
				},
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				log       = mockLogger{}
				transport = mockTransport{}
			)

			i := NewIBFT(log, testCase.backend, transport)
			i.state = testCase.currentState

			// Make sure the message is processed correctly
			assert.ErrorIs(t, i.validateMessage(testCase.message), testCase.expectedErr)
		})
	}
}

func TestIBFT_ValidateMessage_Commit(t *testing.T) {
	t.Parallel()

	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	testTable := []struct {
		name         string
		expectedErr  error
		backend      Backend
		currentState *state
		message      *proto.Message
	}{
		{
			"commit view mismatch",
			errViewMismatch,
			mockBackend{},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_COMMIT,
				// Make sure the view is different
				View: &proto.View{
					Height: 0,
					Round:  1,
				},
			},
		},
		{
			"proposal hash mismatch",
			errHashMismatch,
			mockBackend{
				verifyProposalHashFn: func(_ []byte, _ []byte) error {
					// Make sure the proposal hash is rejected
					return errors.New("invalid hash")
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_COMMIT,
				View: baseView,
				Payload: &proto.Message_CommitData{
					CommitData: &proto.CommitMessage{
						ProposalHash: []byte("proposal hash"),
					},
				},
			},
		},
		{
			"invalid committed seal",
			errInvalidCommittedSeal,
			mockBackend{
				isValidCommittedSealFn: func(_ []byte, _ []byte) bool {
					// Make sure the committed seal is rejected
					return false
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_COMMIT,
				View: baseView,
				Payload: &proto.Message_CommitData{
					CommitData: &proto.CommitMessage{
						CommittedSeal: []byte("committed seal"),
					},
				},
			},
		},
		{
			"valid commit",
			nil,
			mockBackend{
				isValidCommittedSealFn: func(_ []byte, _ []byte) bool {
					return true
				},
			},
			&state{
				view: baseView,
			},
			&proto.Message{
				Type: proto.MessageType_COMMIT,
				View: baseView,
				Payload: &proto.Message_CommitData{
					CommitData: &proto.CommitMessage{
						CommittedSeal: []byte("committed seal"),
					},
				},
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				log       = mockLogger{}
				transport = mockTransport{}
			)

			i := NewIBFT(log, testCase.backend, transport)
			i.state = testCase.currentState

			// Make sure the message is processed correctly
			assert.ErrorIs(t, i.validateMessage(testCase.message), testCase.expectedErr)
		})
	}
}

func TestIBFT_ValidateMessage_RoundChange(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name        string
		expectedErr error
		currentView *proto.View
		message     *proto.Message
	}{
		{
			"height mismatch",
			errViewMismatch,
			&proto.View{
				Height: 0,
				Round:  0,
			},
			&proto.Message{
				View: &proto.View{
					// Make sure the height is different
					Height: 1,
					Round:  0,
				},
				Type: proto.MessageType_ROUND_CHANGE,
			},
		},
		{
			"valid round change message",
			nil,
			&proto.View{
				Height: 0,
				Round:  0,
			},
			&proto.Message{
				View: &proto.View{
					// Make sure the height is the same
					Height: 0,
					Round:  0,
				},
				Type: proto.MessageType_ROUND_CHANGE,
			},
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
			i.state.view = testCase.currentView

			// Make sure the message is processed correctly
			assert.ErrorIs(t, i.validateMessage(testCase.message), testCase.expectedErr)
		})
	}
}

func TestIBFT_AddMessage(t *testing.T) {
	t.Parallel()

	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	testTable := []struct {
		name          string
		message       *proto.Message
		backend       Backend
		currentView   *proto.View
		shouldBeAdded bool
	}{
		{
			"malformed message",
			nil,
			mockBackend{},
			baseView,
			false,
		},
		{
			"invalid message",
			&proto.Message{},
			mockBackend{
				isValidMessageFn: func(message *proto.Message) bool {
					return false
				},
			},
			baseView,
			false,
		},
		{
			"valid message",
			&proto.Message{
				View: baseView,
			},
			mockBackend{
				isValidMessageFn: func(message *proto.Message) bool {
					return true
				},
			},
			baseView,
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				wg              sync.WaitGroup
				receivedMessage *proto.Message = nil
				log                            = mockLogger{}
				transport                      = mockTransport{}
			)

			i := NewIBFT(log, testCase.backend, transport)

			// If the message should be added,
			// make sure the event handler is alerted
			if testCase.shouldBeAdded {
				wg.Add(1)
				go func() {
					defer wg.Done()

					select {
					case message := <-i.newMessageCh:
						receivedMessage = message
					case <-time.After(5 * time.Second):
						return
					}
				}()
			}

			// Add the message
			i.AddMessage(testCase.message)

			wg.Wait()

			if testCase.shouldBeAdded {
				// Make sure the correct message was received
				assert.Equal(t, testCase.message, receivedMessage)
			} else {
				// Make sure no message was sent out
				assert.Nil(t, receivedMessage)
			}
		})
	}
}
