package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Trapesys/go-ibft/messages"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"

	"github.com/Trapesys/go-ibft/messages/proto"
)

func proposalMatches(proposal []byte, message *proto.Message) bool {
	if message == nil || message.Type != proto.MessageType_PREPREPARE {
		return false
	}

	preprepareData, _ := message.Payload.(*proto.Message_PreprepareData)
	extractedProposal := preprepareData.PreprepareData.Proposal

	return bytes.Equal(proposal, extractedProposal)
}

func prepareHashMatches(prepareHash []byte, message *proto.Message) bool {
	if message == nil || message.Type != proto.MessageType_PREPARE {
		return false
	}

	prepareData, _ := message.Payload.(*proto.Message_PrepareData)
	extractedPrepareHash := prepareData.PrepareData.ProposalHash

	return bytes.Equal(prepareHash, extractedPrepareHash)
}

func commitHashMatches(commitHash []byte, message *proto.Message) bool {
	if message == nil || message.Type != proto.MessageType_COMMIT {
		return false
	}

	commitData, _ := message.Payload.(*proto.Message_CommitData)
	extractedCommitHash := commitData.CommitData.ProposalHash

	return bytes.Equal(commitHash, extractedCommitHash)
}

func generateMessages(count uint64, messageType proto.MessageType) []*proto.Message {
	messages := make([]*proto.Message, count)

	for index := uint64(0); index < count; index++ {
		messages[index] = &proto.Message{
			View: &proto.View{
				Height: 0,
				Round:  0,
			},
			Type: messageType,
		}
	}

	return messages
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

			ctx, cancelFn := context.WithCancel(context.Background())

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
				messages = mockMessages{
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						cancelFn()

						return messages.NewSubscription(messages.SubscriptionID(1), make(chan uint64))
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages

			i.wg.Add(1)
			i.runRound(ctx)

			i.wg.Wait()

			// Make sure the node is in prepare state
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
				wg            sync.WaitGroup
				capturedRound uint64 = 0
				ctx, cancelFn        = context.WithCancel(context.Background())

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

			wg.Add(1)
			go func(i *IBFT) {
				defer func() {
					wg.Done()

					cancelFn()
				}()
				select {
				case nextRound := <-i.roundTimer:
					capturedRound = nextRound
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			i.wg.Add(1)
			i.runRound(ctx)

			i.wg.Wait()
			wg.Wait()

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

			ctx, cancelFn := context.WithCancel(context.Background())

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
				messages = mockMessages{
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						cancelFn()

						return messages.NewSubscription(messages.SubscriptionID(1), make(chan uint64))
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages
			i.state.locked = true
			i.state.proposal = previousProposal

			i.wg.Add(1)
			i.runRound(ctx)

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

			ctx, cancelFn := context.WithCancel(context.Background())

			var (
				proposal                          = []byte("new block")
				proposalHash                      = []byte("proposal hash")
				proposer                          = []byte("proposer")
				multicastedPrepare *proto.Message = nil
				notifyCh                          = make(chan uint64, 1)

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
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						return messages.NewSubscription(messages.SubscriptionID(1), notifyCh)
					},
					unsubscribeFn: func(_ messages.SubscriptionID) {
						cancelFn()
					},
					getValidMessagesFn: func(
						view *proto.View,
						_ proto.MessageType,
						isValid func(message *proto.Message) bool,
					) []*proto.Message {
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
			notifyCh <- 0

			i.wg.Add(1)
			i.runRound(ctx)

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

			ctx, cancelFn := context.WithCancel(context.Background())

			var (
				wg            sync.WaitGroup
				capturedRound uint64 = 0
				proposer             = []byte("proposer")
				notifyCh             = make(chan uint64, 1)

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
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						return messages.NewSubscription(messages.SubscriptionID(1), notifyCh)
					},
					getValidMessagesFn: func(
						view *proto.View,
						_ proto.MessageType,
						isValid func(message *proto.Message) bool,
					) []*proto.Message {
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

			wg.Add(1)
			go func(i *IBFT) {
				defer func() {
					wg.Done()

					cancelFn()
				}()

				select {
				case newRound := <-i.roundTimer:
					capturedRound = newRound
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			// Make sure the notification is sent out
			notifyCh <- 0

			i.wg.Add(1)
			i.runRound(ctx)

			i.wg.Wait()
			wg.Wait()

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

			ctx, cancelFn := context.WithCancel(context.Background())

			var (
				proposal                         = []byte("block proposal")
				proposalHash                     = []byte("proposal hash")
				multicastedCommit *proto.Message = nil
				notifyCh                         = make(chan uint64, 1)

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
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						return messages.NewSubscription(messages.SubscriptionID(1), notifyCh)
					},
					unsubscribeFn: func(_ messages.SubscriptionID) {
						cancelFn()
					},
					getValidMessagesFn: func(
						view *proto.View,
						_ proto.MessageType,
						isValid func(message *proto.Message) bool,
					) []*proto.Message {
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
			i.messages = &messages

			// Make sure the notification is present
			notifyCh <- 0

			i.wg.Add(1)
			i.runRound(ctx)

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
				notifyCh                        = make(chan uint64, 1)

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
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						return messages.NewSubscription(messages.SubscriptionID(1), notifyCh)
					},
					getValidMessagesFn: func(
						view *proto.View,
						_ proto.MessageType,
						isValid func(message *proto.Message) bool,
					) []*proto.Message {
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

			ctx, cancelFn := context.WithCancel(context.Background())

			go func(i *IBFT) {
				defer func() {
					cancelFn()
				}()

				select {
				case <-i.roundDone:
					doneReceived = true
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			// Make sure the notification is ready
			notifyCh <- 0

			i.wg.Add(1)
			i.runRound(ctx)

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

			ctx, cancelFn := context.WithCancel(context.Background())

			go func(i *IBFT) {
				defer cancelFn()

				select {
				case newRound := <-i.roundTimer:
					capturedRound = newRound
				case <-time.After(5 * time.Second):
					return
				}
			}(i)

			i.wg.Add(1)
			i.runRound(ctx)

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

		ctx, cancelFn := context.WithCancel(context.Background())

		wg.Add(1)
		i.wg.Add(1)
		go func() {
			i.startRoundTimer(ctx, 0, roundZeroTimeout)

			wg.Done()
		}()

		cancelFn()

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

		ctx, cancelFn := context.WithCancel(context.Background())

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()

				cancelFn()
			}()

			select {
			case newRound := <-i.roundTimer:
				capturedRound = newRound
			case <-time.After(5 * time.Second):
			}
		}()

		i.wg.Add(1)
		i.startRoundTimer(ctx, 0, 0*time.Second)

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
					messages := generateMessages(maxFaultyNodes+1, proto.MessageType_ROUND_CHANGE)

					for i := uint64(0); i < maxFaultyNodes; i++ {
						messages[i].View.Round = suggestedRound
					}

					return messages
				},
			}
		)

		ctx, cancelFn := context.WithCancel(context.Background())

		i := NewIBFT(log, backend, transport)
		i.messages = messages

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()

				cancelFn()
			}()

			select {
			case newRound := <-i.roundTimer:
				capturedRound = newRound
			case <-time.After(5 * time.Second):
			}
		}()

		i.wg.Add(1)
		i.watchForRoundHop(ctx)

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
					messages := generateMessages(maxFaultyNodes, proto.MessageType_ROUND_CHANGE)

					for i := uint64(0); i < maxFaultyNodes; i++ {
						messages[i].View.Round = suggestedRound
					}

					return messages
				},
			}
		)

		ctx, cancelFn := context.WithCancel(context.Background())

		i := NewIBFT(log, backend, transport)
		i.messages = &messages

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
			}()

			select {
			case newRound := <-i.roundTimer:
				capturedRound = newRound
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
			}()

			i.wg.Add(1)
			i.watchForRoundHop(ctx)
		}()

		cancelFn()

		wg.Wait()

		// Make sure the proper round hop number was emitted
		assert.Equal(t, uint64(0), capturedRound)
	})
}

// TestIBFT_MoveToNewRound makes sure the state is modified
// correctly during round moves
func TestIBFT_MoveToNewRound(t *testing.T) {
	t.Parallel()

	t.Run("move to new round", func(t *testing.T) {
		t.Parallel()

		var (
			newRound uint64 = 1

			log       = mockLogger{}
			transport = mockTransport{}
			backend   = mockBackend{}
		)

		i := NewIBFT(log, backend, transport)

		i.moveToNewRound(newRound)

		// Make sure the view has changed
		assert.Equal(t, newRound, i.state.getRound())

		// Make sure the state is unlocked
		assert.False(t, i.state.locked)

		// Make sure the proposal is not present
		assert.Nil(t, i.state.proposal)

		// Make sure the state is correct
		assert.Equal(t, roundChange, i.state.name)
	})

	t.Run("move to new round with RC", func(t *testing.T) {
		t.Parallel()

		var (
			newRound           uint64         = 1
			multicastedMessage *proto.Message = nil

			log       = mockLogger{}
			transport = mockTransport{
				multicastFn: func(message *proto.Message) {
					if message != nil && message.Type == proto.MessageType_ROUND_CHANGE {
						multicastedMessage = message
					}
				},
			}
			backend = mockBackend{}
		)

		i := NewIBFT(log, backend, transport)

		i.moveToNewRoundWithRC(newRound)

		// Make sure the view has changed
		assert.Equal(t, newRound, i.state.getRound())

		// Make sure the state is unlocked
		assert.False(t, i.state.locked)

		// Make sure the proposal is not present
		assert.Nil(t, i.state.proposal)

		// Make sure the state is correct
		assert.Equal(t, roundChange, i.state.name)

		if multicastedMessage == nil {
			t.Fatalf("message not multicasted")
		}

		// Make sure the multicasted message is correct
		assert.Equal(t, newRound, multicastedMessage.View.Round)
		assert.Equal(t, uint64(0), multicastedMessage.View.Height)
	})
}

// TestIBFT_FutureProposal checks the
// behavior when new proposal messages appear
func TestIBFT_FutureProposal(t *testing.T) {
	t.Parallel()

	nodeID := []byte("node ID")
	proposer := []byte("proposer")
	proposal := []byte("proposal")
	proposalHash := []byte("proposal hash")
	quorum := uint64(4)

	generateEmptyRCMessages := func(count uint64) []*proto.Message {
		// Generate random RC messages
		roundChangeMessages := generateMessages(count, proto.MessageType_ROUND_CHANGE)

		// Fill up their certificates
		for _, message := range roundChangeMessages {
			message.Payload = &proto.Message_RoundChangeData{
				RoundChangeData: &proto.RoundChangeMessage{
					LastPreparedProposedBlock: nil,
					LatestPreparedCertificate: nil,
				},
			}
		}

		return roundChangeMessages
	}

	generateFilledRCMessages := func(count uint64) []*proto.Message {
		// Generate random RC messages
		roundChangeMessages := generateMessages(quorum, proto.MessageType_ROUND_CHANGE)
		prepareMessages := generateMessages(quorum-1, proto.MessageType_PREPARE)

		// Fill up the prepare message hashes
		for index, message := range prepareMessages {
			message.Payload = &proto.Message_PrepareData{
				PrepareData: &proto.PrepareMessage{
					ProposalHash: proposalHash,
				},
			}
			message.View = &proto.View{
				Height: 0,
				Round:  1,
			}
			message.From = []byte(fmt.Sprintf("node %d", index+1))
		}

		lastPreparedCertificate := &proto.PreparedCertificate{
			ProposalMessage: &proto.Message{
				View: &proto.View{
					Height: 0,
					Round:  1,
				},
				From: []byte("unique node"),
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal:     proposal,
						ProposalHash: proposalHash,
						Certificate:  nil,
					},
				},
			},
			PrepareMessages: prepareMessages,
		}

		// Fill up their certificates
		for _, message := range roundChangeMessages {
			message.Payload = &proto.Message_RoundChangeData{
				RoundChangeData: &proto.RoundChangeMessage{
					LastPreparedProposedBlock: proposal,
					LatestPreparedCertificate: lastPreparedCertificate,
				},
			}
			message.View = &proto.View{
				Height: 0,
				Round:  1,
			}
		}

		return roundChangeMessages
	}

	testTable := []struct {
		name                string
		proposalView        *proto.View
		roundChangeMessages []*proto.Message
		notifyRound         uint64
	}{
		{
			"valid future proposal with new block",
			&proto.View{
				Height: 0,
				Round:  1,
			},
			generateEmptyRCMessages(quorum),
			1,
		},
		{
			"valid future proposal with old block",
			&proto.View{
				Height: 0,
				Round:  2,
			},
			generateFilledRCMessages(quorum),
			2,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancelFn := context.WithCancel(context.Background())

			validProposal := &proto.Message{
				View: testCase.proposalView,
				From: proposer,
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal:     proposal,
						ProposalHash: proposalHash,
						Certificate: &proto.RoundChangeCertificate{
							RoundChangeMessages: testCase.roundChangeMessages,
						},
					},
				},
			}

			var (
				wg                    sync.WaitGroup
				receivedProposalEvent *newProposalEvent = nil
				notifyCh                                = make(chan uint64, 1)

				log     = mockLogger{}
				backend = mockBackend{
					isProposerFn: func(id []byte, _ uint64, _ uint64) bool {
						return !bytes.Equal(id, nodeID)
					},
					idFn: func() []byte {
						return nodeID
					},
					isValidProposalHashFn: func(p []byte, hash []byte) bool {
						return bytes.Equal(hash, proposalHash) && bytes.Equal(p, proposal)
					},
					quorumFn: func(_ uint64) uint64 {
						return quorum
					},
				}
				transport = mockTransport{}
				messages  = mockMessages{
					subscribeFn: func(_ messages.SubscriptionDetails) *messages.Subscription {
						return messages.NewSubscription(messages.SubscriptionID(1), notifyCh)
					},
					getValidMessagesFn: func(
						view *proto.View,
						_ proto.MessageType,
						isValid func(message *proto.Message) bool,
					) []*proto.Message {
						return filterMessages(
							[]*proto.Message{
								validProposal,
							},
							isValid,
						)
					},
				}
			)

			i := NewIBFT(log, backend, transport)
			i.messages = messages

			wg.Add(1)
			go func() {
				defer func() {
					cancelFn()

					wg.Done()
				}()

				select {
				case <-time.After(5 * time.Second):
				case event := <-i.newProposal:
					receivedProposalEvent = &event
				}
			}()

			notifyCh <- testCase.notifyRound

			i.wg.Add(1)
			i.watchForFutureProposal(ctx)

			wg.Wait()

			// Make sure the received proposal is the one that was expected
			if receivedProposalEvent == nil {
				t.Fatalf("no proposal event received")
			}

			assert.Equal(t, testCase.notifyRound, receivedProposalEvent.round)
			assert.Equal(t, proposal, receivedProposalEvent.proposal)
		})
	}
}
