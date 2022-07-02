package core

import (
	"bytes"
	"fmt"
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"testing"
)

// generateNodeAddresses generates dummy node addresses
func generateNodeAddresses(count int) [][]byte {
	addresses := make([][]byte, count)

	for index := 0; index < count; index++ {
		addresses[index] = []byte(fmt.Sprintf("node %d", index))
	}

	return addresses
}

// buildBasicPreprepareMessage builds a simple preprepare message
func buildBasicPreprepareMessage(
	proposal,
	from []byte,
	view *proto.View,
) *proto.Message {
	return &proto.Message{
		View: view,
		From: from,
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal: proposal,
			},
		},
	}
}

// buildBasicPrepareMessage builds a simple prepare message
func buildBasicPrepareMessage(
	proposalHash,
	from []byte,
	view *proto.View,
) *proto.Message {
	return &proto.Message{
		View: view,
		From: from,
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}
}

// buildBasicCommitMessage builds a simple commit message
func buildBasicCommitMessage(
	proposalHash,
	committedSeal,
	from []byte,
	view *proto.View,
) *proto.Message {
	return &proto.Message{
		View: view,
		From: from,
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}
}

// buildBasicRoundChangeMessage builds a simple round change message
func buildBasicRoundChangeMessage(
	height,
	round uint64,
	from []byte,
) *proto.Message {
	return &proto.Message{
		View: &proto.View{
			Height: height,
			Round:  round,
		},
		From:    from,
		Type:    proto.MessageType_ROUND_CHANGE,
		Payload: nil,
	}
}

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
	numNodes := 4
	nodes := generateNodeAddresses(numNodes)
	insertedBlocks := make([][]byte, numNodes)

	// commonTransportCallback is the common method modification
	// required for Transport, for all nodes
	commonTransportCallback := func(transport *mockTransport) {
		transport.multicastFn = func(message *proto.Message) {
			multicastFn(message)
		}
	}

	// commonBackendCallback is the common method modification required
	// for the Backend, for all nodes
	commonBackendCallback := func(backend *mockBackend, nodeIndex int) {
		// Make sure the quorum function requires all nodes
		backend.quorumFn = func(_ uint64) uint64 {
			return uint64(numNodes)
		}

		// Make sure the only proposer is node 0
		backend.isProposerFn = func(from []byte, _ uint64, _ uint64) bool {
			return bytes.Equal(from, nodes[0])
		}

		// Make sure the proposal is valid if it matches what node 0 proposed
		backend.isValidBlockFn = func(newProposal []byte) bool {
			return bytes.Equal(newProposal, proposal)
		}

		// Make sure the preprepare message is built correctly
		backend.buildPrePrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
			return buildBasicPreprepareMessage(proposal, nodes[nodeIndex], view)
		}

		// Make sure the prepare message is built correctly
		backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
			return buildBasicPrepareMessage(proposalHash, nodes[nodeIndex], view)
		}

		// Make sure the commit message is built correctly
		backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
			return buildBasicCommitMessage(proposalHash, committedSeal, nodes[nodeIndex], view)
		}

		// Make sure the round change message is built correctly
		backend.buildRoundChangeMessageFn = func(height uint64, round uint64) *proto.Message {
			return buildBasicRoundChangeMessage(height, round, nodes[nodeIndex])
		}

		// Make sure the inserted proposal is noted
		backend.insertBlockFn = func(proposal []byte, committedSeals [][]byte) error {
			insertedBlocks[nodeIndex] = proposal

			return nil
		}
	}

	var (
		backendCallbackMap = map[int]backendConfigCallback{
			0: func(backend *mockBackend) {
				// Execute the common backend setup
				commonBackendCallback(backend, 0)

				// Set the proposal creation method for node 0, since
				// they are the proposer
				backend.buildProposalFn = func(u uint64) ([]byte, error) {
					return proposal, nil
				}

				// Make sure node 0 knows they are the proposer for this round
				backend.isProposerFn = func(_ []byte, _ uint64, _ uint64) bool {
					return true
				}
			},
			1: func(backend *mockBackend) {
				commonBackendCallback(backend, 1)
			},
			2: func(backend *mockBackend) {
				commonBackendCallback(backend, 2)
			},
			3: func(backend *mockBackend) {
				commonBackendCallback(backend, 3)
			},
		}
		transportCallbackMap = map[int]transportConfigCallback{
			0: commonTransportCallback,
			1: commonTransportCallback,
			2: commonTransportCallback,
			3: commonTransportCallback,
		}
	)

	// Create the mock cluster
	cluster := newMockCluster(
		numNodes,
		backendCallbackMap,
		nil,
		transportCallbackMap,
	)

	// Set the multicast callback to relay the message
	// to the entire cluster
	multicastFn = func(message *proto.Message) {
		cluster.pushMessage(message)
	}

	// Start the main run loops
	cluster.runNewRound()

	// Wait until the main run loops finish
	cluster.stop()

	// Make sure the inserted blocks match what node 0 proposed
	for _, block := range insertedBlocks {
		assert.True(t, bytes.Equal(block, proposal))
	}
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
