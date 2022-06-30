package core

import (
	"bytes"
	"fmt"
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// TestConsensus_ValidFlow tests the following scenario:
// N = 4
//
// - Node 0 is the proposer for block 1, round 0
// - Node 0 proposes a valid block B
// - All nodes go through the consensus states to insert the valid block B
func TestConsensus_ValidFlow(t *testing.T) {
	var multicastFn func(message *proto.Message)

	proposal := []byte("proposal")
	proposalHash := []byte("proposal hash")
	committedSeal := []byte("seal")
	nodes := [][]byte{
		[]byte("node 0"),
		[]byte("node 1"),
		[]byte("node 2"),
		[]byte("node 3"),
	}
	insertedBlocks := make([][]byte, 4)

	defaultTransportCallback := func(transport *mockTransport) {
		transport.multicastFn = func(message *proto.Message) {
			multicastFn(message)
		}
	}

	quorumFn := func(blockHeight uint64) uint64 {
		return 4
	}

	buildPrepareMessage := func(proposal []byte, view *proto.View) *proto.Message {
		return &proto.Message{
			View: view,
			Type: proto.MessageType_PREPARE,
			Payload: &proto.Message_PrepareData{
				PrepareData: &proto.PrepareMessage{
					ProposalHash: proposalHash,
				},
			},
		}
	}

	buildCommitMessage := func(proposal []byte, view *proto.View) *proto.Message {
		return &proto.Message{
			View: view,
			Type: proto.MessageType_COMMIT,
			Payload: &proto.Message_CommitData{
				CommitData: &proto.CommitMessage{
					ProposalHash:  proposalHash,
					CommittedSeal: committedSeal,
				},
			},
		}
	}

	buildPreprepareMessage := func(proposal []byte, view *proto.View) *proto.Message {
		return &proto.Message{
			View: view,
			Type: proto.MessageType_PREPREPARE,
			Payload: &proto.Message_PreprepareData{
				PreprepareData: &proto.PrePrepareMessage{
					Proposal: proposal,
				},
			},
		}
	}

	buildRoundChangeMessage := func(height, round uint64) *proto.Message {
		return &proto.Message{
			View: &proto.View{
				Height: height,
				Round:  round,
			},
			Type:    proto.MessageType_ROUND_CHANGE,
			Payload: nil,
		}
	}

	var (
		logCallbackMap = map[int]loggerConfigCallback{
			0: func(logger *mockLogger) {
				logger.infoFn = func(s string, i ...interface{}) {
					fmt.Printf("[Node 0]: %s\n", s)
				}
			},
			1: func(logger *mockLogger) {
				logger.infoFn = func(s string, i ...interface{}) {
					fmt.Printf("[Node 1]: %s\n", s)
				}
			},
			2: func(logger *mockLogger) {
				logger.infoFn = func(s string, i ...interface{}) {
					fmt.Printf("[Node 2]: %s\n", s)
				}
			},
			3: func(logger *mockLogger) {
				logger.infoFn = func(s string, i ...interface{}) {
					fmt.Printf("[Node 3]: %s\n", s)
				}
			},
		}
		backendCallbackMap = map[int]backendConfigCallback{
			0: func(backend *mockBackend) {
				backend.buildProposalFn = func(u uint64) ([]byte, error) {
					return proposal, nil
				}

				backend.isProposerFn = func(_ []byte, _ uint64, _ uint64) bool {
					return true
				}

				backend.isValidBlockFn = func(newProposal []byte) bool {
					return bytes.Equal(newProposal, proposal)
				}

				backend.quorumFn = quorumFn

				backend.buildPrePrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildPreprepareMessage(proposal, view)

					message.From = nodes[0]

					return message
				}
				backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildPrepareMessage(proposal, view)

					message.From = nodes[0]

					return message
				}
				backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildCommitMessage(proposal, view)

					message.From = nodes[0]

					return message
				}
				backend.buildRoundChangeMessageFn = func(height uint64, round uint64) *proto.Message {
					message := buildRoundChangeMessage(height, round)

					message.From = nodes[0]

					return message
				}

				backend.insertBlockFn = func(proposal []byte, committedSeals [][]byte) error {
					insertedBlocks[0] = proposal

					return nil
				}
			},
			1: func(backend *mockBackend) {
				backend.quorumFn = quorumFn

				backend.isProposerFn = func(from []byte, _ uint64, _ uint64) bool {
					return bytes.Equal(from, nodes[0])
				}

				backend.isValidBlockFn = func(newProposal []byte) bool {
					return bytes.Equal(newProposal, proposal)
				}

				backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildPrepareMessage(proposal, view)

					message.From = nodes[1]

					return message
				}
				backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildCommitMessage(proposal, view)

					message.From = nodes[1]

					return message
				}
				backend.buildRoundChangeMessageFn = func(height uint64, round uint64) *proto.Message {
					message := buildRoundChangeMessage(height, round)

					message.From = nodes[1]

					return message
				}

				backend.insertBlockFn = func(proposal []byte, committedSeals [][]byte) error {
					insertedBlocks[1] = proposal

					return nil
				}
			},
			2: func(backend *mockBackend) {
				backend.quorumFn = quorumFn

				backend.isProposerFn = func(from []byte, _ uint64, _ uint64) bool {
					return bytes.Equal(from, nodes[0])
				}

				backend.isValidBlockFn = func(newProposal []byte) bool {
					return bytes.Equal(newProposal, proposal)
				}

				backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildPrepareMessage(proposal, view)

					message.From = nodes[2]

					return message
				}
				backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildCommitMessage(proposal, view)

					message.From = nodes[2]

					return message
				}
				backend.buildRoundChangeMessageFn = func(height uint64, round uint64) *proto.Message {
					message := buildRoundChangeMessage(height, round)

					message.From = nodes[2]

					return message
				}

				backend.insertBlockFn = func(proposal []byte, committedSeals [][]byte) error {
					insertedBlocks[2] = proposal

					return nil
				}
			},
			3: func(backend *mockBackend) {
				backend.quorumFn = quorumFn

				backend.isProposerFn = func(from []byte, _ uint64, _ uint64) bool {
					return bytes.Equal(from, nodes[0])
				}

				backend.isValidBlockFn = func(newProposal []byte) bool {
					return bytes.Equal(newProposal, proposal)
				}

				backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildPrepareMessage(proposal, view)

					message.From = nodes[3]

					return message
				}
				backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
					message := buildCommitMessage(proposal, view)

					message.From = nodes[3]

					return message
				}
				backend.buildRoundChangeMessageFn = func(height uint64, round uint64) *proto.Message {
					message := buildRoundChangeMessage(height, round)

					message.From = nodes[3]

					return message
				}

				backend.insertBlockFn = func(proposal []byte, committedSeals [][]byte) error {
					insertedBlocks[3] = proposal

					return nil
				}
			},
		}
		transportCallbackMap = map[int]transportConfigCallback{
			0: defaultTransportCallback,
			1: defaultTransportCallback,
			2: defaultTransportCallback,
			3: defaultTransportCallback,
		}
	)

	cluster := newMockCluster(
		4,
		backendCallbackMap,
		logCallbackMap,
		transportCallbackMap,
	)

	multicastFn = func(message *proto.Message) {
		cluster.pushMessage(message)
	}

	cluster.runNewRound()

	// TODO figure out a better way to catch the end event
	select {
	case <-time.After(time.Second * 2):
	}
	cluster.stop()

	for _, block := range insertedBlocks {
		assert.True(t, bytes.Equal(block, proposal))
	}

	fmt.Println("🎉 Consensus reached 🎉")
}

// TestConsensus_InvalidBlock tests the following scenario:
// N = 4
//
// - Node 0 is the proposer for block 1, round 0
// - Node 0 proposes an invalid block B
// - Other nodes should verify that the block is invalid
// - All nodes should move to round 1, and start a new consensus round
func TestConsensus_InvalidBlock(t *testing.T) {
	// TODO implement
}

// TestConsensus_Persistence verifies the persistence problem
// outlined in the following analysis paper:
// https://arxiv.org/pdf/1901.07160.pdf
func TestConsensus_Persistence(t *testing.T) {
	// TODO implement
}

// TestConsensus_Liveness verifies the liveness problem
// outlined in the following analysis paper:
// https://arxiv.org/pdf/1901.07160.pdf
func TestConsensus_Liveness(t *testing.T) {
	// TODO implement
}
