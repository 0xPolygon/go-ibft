package core

import (
	"bytes"
	"errors"
	"github.com/Trapesys/go-ibft/messages"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"

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

func generatePrepareMessages(count uint64) []*proto.Message {
	prepares := make([]*proto.Message, count)

	for index := uint64(0); index < count; index++ {
		prepares[index] = &proto.Message{
			Payload: &proto.Message_PrepareData{
				PrepareData: &proto.PrepareMessage{
					ProposalHash: nil,
				},
			},
		}
	}

	return prepares
}

func generateCommitMessages(count uint64) []*proto.Message {
	commits := make([]*proto.Message, count)

	for index := uint64(0); index < count; index++ {
		commits[index] = &proto.Message{
			Payload: &proto.Message_CommitData{
				CommitData: &proto.CommitMessage{
					ProposalHash: nil,
				},
			},
		}
	}

	return commits
}

func generateRoundChangeMessages(count uint64) []*proto.Message {
	roundChanges := make([]*proto.Message, count)

	for index := uint64(0); index < count; index++ {
		roundChanges[index] = &proto.Message{
			View: &proto.View{
				Height: 0,
				Round:  0,
			},
			Type: proto.MessageType_ROUND_CHANGE,
		}
	}

	return roundChanges
}

func generateSeals(count int) [][]byte {
	seals := make([][]byte, count)

	for i := 0; i < count; i++ {
		seals[i] = []byte("committed seal")
	}

	return seals
}

func filterMessages(messages []*proto.Message, isValid func(message *proto.Message) bool) []*proto.Message {
	newMessages := make([]*proto.Message, 0)

	for _, message := range messages {
		if isValid(message) {
			newMessages = append(newMessages, message)
		}
	}

	return newMessages
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

			i.wg.Wait()

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
				capturedRound uint64 = 0
				quitCh               = make(chan struct{}, 1)

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
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

			go func(i *IBFT) {
				select {
				case nextRound := <-i.roundChange:
					capturedRound = nextRound
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			i.runRound(quitCh)

			i.wg.Wait()

			// Make sure the proposal is not accepted
			assert.Equal(t, []byte(nil), i.state.proposal)

			// Make sure the correct round number was emitted
			assert.Equal(t, uint64(1), capturedRound)
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

			i.wg.Wait()

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
				proposer                          = []byte("proposer")
				multicastedPrepare *proto.Message = nil
				notifyCh                          = make(chan struct{}, 1)

				log       = mockLogger{}
				transport = mockTransport{
					func(message *proto.Message) {
						if message != nil && message.Type == proto.MessageType_PREPARE {
							multicastedPrepare = message
						}
					},
				}
				backend = mockBackend{
					idFn: func() []byte {
						return []byte("non proposer")
					},
					quorumFn: func(_ uint64) uint64 {
						return 1
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
					isProposerFn: func(from []byte, _, _ uint64) bool {
						return bytes.Equal(from, proposer)
					},
					isValidBlockFn: func(_ []byte) bool {
						return true
					},
				}
				messages = mockMessages{
					subscribeFn: func(_ messages.Subscription) *messages.SubscribeResult {
						return messages.NewSubscribeResult(messages.SubscriptionID(1), notifyCh)
					},
					unsubscribeFn: func(_ messages.SubscriptionID) {
						quitCh <- struct{}{}
					},
					getValidMessagesFn: func(view *proto.View, _ proto.MessageType, isValid func(message *proto.Message) bool) []*proto.Message {
						return filterMessages(
							[]*proto.Message{
								{
									View: view,
									From: proposer,
									Type: proto.MessageType_PREPREPARE,
									Payload: &proto.Message_PreprepareData{
										PreprepareData: &proto.PrePrepareMessage{
											Proposal: proposal,
										},
									},
								},
							},
							isValid,
						)
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages

			// Make sure the notification is sent out
			notifyCh <- struct{}{}

			i.runRound(quitCh)

			i.wg.Wait()

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
				capturedRound uint64 = 0
				proposer             = []byte("proposer")
				notifyCh             = make(chan struct{}, 1)

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					idFn: func() []byte {
						return nil
					},
					quorumFn: func(_ uint64) uint64 {
						return 1
					},
					isProposerFn: func(from []byte, _ uint64, _ uint64) bool {
						return bytes.Equal(from, proposer)
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
				messages = mockMessages{
					subscribeFn: func(_ messages.Subscription) *messages.SubscribeResult {
						return messages.NewSubscribeResult(messages.SubscriptionID(1), notifyCh)
					},
					unsubscribeFn: func(_ messages.SubscriptionID) {
						quitCh <- struct{}{}
					},
					getValidMessagesFn: func(view *proto.View, _ proto.MessageType, isValid func(message *proto.Message) bool) []*proto.Message {
						return filterMessages(
							[]*proto.Message{
								{
									View: view,
									From: proposer,
									Type: proto.MessageType_PREPREPARE,
									Payload: &proto.Message_PreprepareData{
										PreprepareData: &proto.PrePrepareMessage{
											Proposal: nil,
										},
									},
								},
							},
							isValid,
						)
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages

			go func(i *IBFT) {
				select {
				case newRound := <-i.roundChange:
					capturedRound = newRound
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			// Make sure the notification is sent out
			notifyCh <- struct{}{}

			i.runRound(quitCh)

			i.wg.Wait()

			// Make sure the proposal is not accepted
			assert.Equal(t, []byte(nil), i.state.proposal)

			// Make sure the correct error was emitted
			assert.Equal(t, uint64(1), capturedRound)
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
				notifyCh                         = make(chan struct{}, 1)

				log       = mockLogger{}
				transport = mockTransport{func(message *proto.Message) {
					if message != nil && message.Type == proto.MessageType_COMMIT {
						multicastedCommit = message
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
					quorumFn: func(_ uint64) uint64 {
						return 1
					},
					isValidProposalHashFn: func(_ []byte, hash []byte) bool {
						return bytes.Equal(proposalHash, hash)
					},
				}
				messages = mockMessages{
					subscribeFn: func(_ messages.Subscription) *messages.SubscribeResult {
						return messages.NewSubscribeResult(messages.SubscriptionID(1), notifyCh)
					},
					unsubscribeFn: func(_ messages.SubscriptionID) {
						quitCh <- struct{}{}
					},
					getValidMessagesFn: func(view *proto.View, _ proto.MessageType, isValid func(message *proto.Message) bool) []*proto.Message {
						return filterMessages(
							[]*proto.Message{
								{
									View: view,
									Type: proto.MessageType_PREPARE,
									Payload: &proto.Message_PrepareData{
										PrepareData: &proto.PrepareMessage{
											ProposalHash: proposalHash,
										},
									},
								},
							},
							isValid,
						)
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.state.name = prepare
			i.state.roundStarted = true
			i.state.proposal = proposal
			i.messages = messages

			// Make sure the notification is present
			notifyCh <- struct{}{}

			i.runRound(quitCh)

			i.wg.Wait()

			// Make sure the node moves to the commit state
			assert.Equal(t, commit, i.state.name)

			// Make sure the node stays locked on a block
			assert.True(t, i.state.locked)

			// Make sure the proposal didn't change
			assert.Equal(t, proposal, i.state.proposal)

			// Make sure the proper proposal hash was multicasted
			assert.True(t, commitHashMatches(proposalHash, multicastedCommit))
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

			var (
				proposal                        = []byte("block proposal")
				proposalHash                    = []byte("proposal hash")
				insertedProposal       []byte   = nil
				insertedCommittedSeals [][]byte = nil
				committedSeals                  = generateSeals(1)
				doneReceived                    = false
				notifyCh                        = make(chan struct{}, 1)

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					insertBlockFn: func(proposal []byte, committedSeals [][]byte) error {
						insertedProposal = proposal
						insertedCommittedSeals = committedSeals

						return nil
					},
					quorumFn: func(_ uint64) uint64 {
						return 1
					},
					isValidProposalHashFn: func(_ []byte, hash []byte) bool {
						return bytes.Equal(proposalHash, hash)
					},
				}
				messages = mockMessages{
					subscribeFn: func(_ messages.Subscription) *messages.SubscribeResult {
						return messages.NewSubscribeResult(messages.SubscriptionID(1), notifyCh)
					},
					getValidMessagesFn: func(view *proto.View, _ proto.MessageType, isValid func(message *proto.Message) bool) []*proto.Message {
						return filterMessages(
							[]*proto.Message{
								{
									View: view,
									Type: proto.MessageType_COMMIT,
									Payload: &proto.Message_CommitData{
										CommitData: &proto.CommitMessage{
											ProposalHash:  proposalHash,
											CommittedSeal: committedSeals[0],
										},
									},
								},
							},
							isValid,
						)
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.state.proposal = proposal
			i.state.roundStarted = true
			i.state.name = commit

			quitCh := make(chan struct{}, 1)

			go func(i *IBFT) {
				defer func() {
					quitCh <- struct{}{}
				}()

				select {
				case <-i.roundDone:
					doneReceived = true
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			// Make sure the notification is ready
			notifyCh <- struct{}{}

			i.runRound(quitCh)

			i.wg.Wait()

			// Make sure the node changed the state to fin
			assert.Equal(t, fin, i.state.name)

			// Make sure the inserted proposal was the one present
			assert.Equal(t, insertedProposal, proposal)

			// Make sure the inserted committed seals were correct
			assert.Equal(t, insertedCommittedSeals, committedSeals)

			// Make sure the proper done channel was notified
			assert.True(t, doneReceived)
		},
	)

	t.Run(
		"validator failed to insert the finalized block",
		func(t *testing.T) {
			t.Parallel()

			var (
				capturedRound uint64 = 0

				log       = mockLogger{}
				transport = mockTransport{}
				backend   = mockBackend{
					insertBlockFn: func(proposal []byte, committedSeals [][]byte) error {
						return errInsertBlock
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
			i.messages = mockMessages{}
			i.state.name = fin
			i.state.roundStarted = true

			go func(i *IBFT) {
				select {
				case newRound := <-i.roundChange:
					capturedRound = newRound
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			quitCh := make(chan struct{}, 1)

			i.runRound(quitCh)

			i.wg.Wait()

			// Make sure the captured round change number is captured
			assert.Equal(t, uint64(1), capturedRound)
		},
	)
}

// TestIBFT_IsAcceptableMessage makes sure invalid messages
// are properly handled
func TestIBFT_IsAcceptableMessage(t *testing.T) {
	t.Parallel()

	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	testTable := []struct {
		name          string
		view          *proto.View
		currentView   *proto.View
		invalidSender bool
		acceptable    bool
	}{
		{
			"invalid sender",
			nil,
			baseView,
			true,
			false,
		},
		{
			"malformed message",
			nil,
			baseView,
			false,
			false,
		},
		{
			"higher height number",
			&proto.View{
				Height: baseView.Height + 100,
				Round:  baseView.Round,
			},
			baseView,
			false,
			true,
		},
		{
			"higher round number",
			&proto.View{
				Height: baseView.Height,
				Round:  baseView.Round + 1,
			},
			baseView,
			false,
			true,
		},
		{
			"lower height number",
			baseView,
			&proto.View{
				Height: baseView.Height + 1,
				Round:  baseView.Round,
			},
			false,
			false,
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
					isValidSenderFn: func(message *proto.Message) bool {
						return !testCase.invalidSender
					},
				}
			)
			i := NewIBFT(log, backend, transport)
			i.state.view = testCase.currentView

			message := &proto.Message{
				View: testCase.view,
			}

			assert.Equal(t, testCase.acceptable, i.isAcceptableMessage(message))
		})
	}
}

// TestIBFT_StartRoundTimer makes sure that the
// round timer behaves correctly
func TestIBFT_StartRoundTimer(t *testing.T) {
	t.Parallel()

	t.Run("round timer exits due to a quit signal", func(t *testing.T) {
		t.Parallel()

		var (
			wg sync.WaitGroup

			log       = mockLogger{}
			transport = mockTransport{}
			backend   = mockBackend{}
		)

		i := NewIBFT(log, backend, transport)

		quitCh := make(chan struct{})

		wg.Add(1)
		go func() {
			i.startRoundTimer(0, roundZeroTimeout, quitCh)

			wg.Done()
		}()

		quitCh <- struct{}{}

		wg.Wait()
	})

	t.Run("round timer expires", func(t *testing.T) {
		t.Parallel()

		var (
			wg            sync.WaitGroup
			capturedRound uint64 = 0

			log       = mockLogger{}
			transport = mockTransport{}
			backend   = mockBackend{}
		)

		i := NewIBFT(log, backend, transport)

		quitCh := make(chan struct{}, 1)

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()

				quitCh <- struct{}{}
			}()

			select {
			case newRound := <-i.roundChange:
				capturedRound = newRound
			case <-time.After(5 * time.Second):
			}
		}()

		i.startRoundTimer(0, 0*time.Second, quitCh)

		wg.Wait()

		// Make sure the proper round was emitted
		assert.Equal(t, uint64(1), capturedRound)
	})
}

// TestIBFT_WatchForRoundHop makes sure that the
// watch round hop routine behaves correctly
func TestIBFT_WatchForRoundHop(t *testing.T) {
	t.Parallel()

	t.Run("received F+1 round hop messages", func(t *testing.T) {
		t.Parallel()

		var (
			wg             sync.WaitGroup
			capturedRound  uint64 = 0
			suggestedRound uint64 = 10
			maxFaultyNodes uint64 = 2

			log       = mockLogger{}
			transport = mockTransport{}
			backend   = mockBackend{
				maximumFaultyNodesFn: func() uint64 {
					return maxFaultyNodes
				},
			}
			messages = mockMessages{
				getMostRoundChangeMessagesFn: func(_ uint64, _ uint64) []*proto.Message {
					messages := generateRoundChangeMessages(maxFaultyNodes + 1)

					for i := uint64(0); i < maxFaultyNodes; i++ {
						messages[i].View.Round = suggestedRound
					}

					return messages
				},
			}
		)

		quitCh := make(chan struct{}, 1)

		i := NewIBFT(log, backend, transport)
		i.messages = messages

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()

				quitCh <- struct{}{}
			}()

			select {
			case newRound := <-i.roundChange:
				capturedRound = newRound
			case <-time.After(5 * time.Second):
			}
		}()

		i.watchForRoundHop(quitCh)

		wg.Wait()

		// Make sure the proper round hop number was emitted
		assert.Equal(t, suggestedRound, capturedRound)
	})

	t.Run("received a quit signal", func(t *testing.T) {
		t.Parallel()

		var (
			wg             sync.WaitGroup
			capturedRound  uint64 = 0
			suggestedRound uint64 = 10
			maxFaultyNodes uint64 = 2

			log       = mockLogger{}
			transport = mockTransport{}
			backend   = mockBackend{
				maximumFaultyNodesFn: func() uint64 {
					return maxFaultyNodes
				},
			}
			messages = mockMessages{
				getMostRoundChangeMessagesFn: func(_ uint64, _ uint64) []*proto.Message {
					messages := generateRoundChangeMessages(maxFaultyNodes)

					for i := uint64(0); i < maxFaultyNodes; i++ {
						messages[i].View.Round = suggestedRound
					}

					return messages
				},
			}
		)

		quitCh := make(chan struct{})

		i := NewIBFT(log, backend, transport)
		i.messages = messages

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
			}()

			select {
			case newRound := <-i.roundChange:
				capturedRound = newRound
			case <-time.After(5 * time.Second):
			case <-quitCh:
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
			}()

			i.watchForRoundHop(quitCh)
		}()

		close(quitCh)

		wg.Wait()

		// Make sure the proper round hop number was emitted
		assert.Equal(t, uint64(0), capturedRound)
	})
}
