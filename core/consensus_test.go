package core

import (
	"bytes"
	"fmt"
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"math"
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

		// Make sure the node ID is properly relayed
		backend.idFn = func() []byte {
			return nodes[nodeIndex]
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
	)

	// Create the mock cluster
	cluster := newMockCluster(
		numNodes,
		backendCallbackMap,
		logCallbackMap,
		transportCallbackMap,
	)

	// Set the multicast callback to relay the message
	// to the entire cluster
	multicastFn = func(message *proto.Message) {
		cluster.pushMessage(message)
	}

	// Start the main run loops
	cluster.runSequence(1)

	// Wait until the main run loops finish
	cluster.stop()

	// Make sure the inserted blocks match what node 0 proposed
	for _, block := range insertedBlocks {
		assert.True(t, bytes.Equal(block, proposal))
	}
}

//func TestDummy(t *testing.T) {
//	for i := 0; i < 100000; i++ {
//		fmt.Printf("test run #%d\n", i)
//		TestConsensus_InvalidBlock(t)
//		TestConsensus_ValidFlow(t)
//	}
//
//	fmt.Println("🎉Yeah boy🎉")
//}

// TestConsensus_InvalidBlock tests the following scenario:
// N = 4
//
// - Node 0 is the proposer for block 1, round 0
// - Node 0 proposes an invalid block B
// - Other nodes should verify that the block is invalid
// - All nodes should move to round 1, and start a new consensus round
// - Node 1 is the proposer for block 1, round 1
// - Node 1 proposes a valid block B'
// - All nodes go through the consensus states to insert the valid block B'
func TestConsensus_InvalidBlock(t *testing.T) {
	//t.SkipNow()
	var multicastFn func(message *proto.Message)

	proposals := [][]byte{
		[]byte("proposal 1"), // proposed by node 0
		[]byte("proposal 2"), // proposed by node 1
	}

	proposalHashes := [][]byte{
		[]byte("proposal hash 1"), // for proposal 1
		[]byte("proposal hash 2"), // for proposal 2
	}
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

	maxFaulty := func(nodeCount int) uint64 {
		return uint64((nodeCount - 1) / 3)
	}

	// commonBackendCallback is the common method modification required
	// for the Backend, for all nodes
	commonBackendCallback := func(backend *mockBackend, nodeIndex int) {
		// Make sure the quorum function is Quorum optimal
		backend.quorumFn = func(_ uint64) uint64 {
			if maxFaulty(numNodes) == 0 {
				return uint64(numNodes)
			}

			return uint64(math.Ceil(2 * float64(numNodes) / 3))
		}

		// Make sure the allowed faulty nodes function is accurate
		backend.maximumFaultyNodesFn = func() uint64 {
			return maxFaulty(numNodes)
		}

		// Make sure the node ID is properly relayed
		backend.idFn = func() []byte {
			return nodes[nodeIndex]
		}

		// Make sure the only proposer is node 0
		backend.isProposerFn = func(from []byte, _ uint64, round uint64) bool {
			// Node 0 is the proposer for round 0
			// Node 1 is the proposer for round 1
			return bytes.Equal(from, nodes[round])
		}

		// Make sure the proposal is valid if it matches what node 0 proposed
		backend.isValidBlockFn = func(newProposal []byte) bool {
			// Node 1 is the proposer for round 1,
			// and their proposal is the only one that's valid
			return bytes.Equal(newProposal, proposals[1])
		}

		// Make sure the preprepare message is built correctly
		backend.buildPrePrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
			return buildBasicPreprepareMessage(proposal, nodes[nodeIndex], view)
		}

		// Make sure the prepare message is built correctly
		backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
			return buildBasicPrepareMessage(proposalHashes[view.Round], nodes[nodeIndex], view)
		}

		// Make sure the commit message is built correctly
		backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
			return buildBasicCommitMessage(proposalHashes[view.Round], committedSeal, nodes[nodeIndex], view)
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
				commonBackendCallback(backend, 0)

				backend.buildProposalFn = func(_ uint64) ([]byte, error) {
					return proposals[0], nil
				}
			},
			1: func(backend *mockBackend) {
				commonBackendCallback(backend, 1)

				backend.buildProposalFn = func(_ uint64) ([]byte, error) {
					return proposals[1], nil
				}
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
	)

	// Create the mock cluster
	cluster := newMockCluster(
		numNodes,
		backendCallbackMap,
		logCallbackMap,
		transportCallbackMap,
	)

	// Set the multicast callback to relay the message
	// to the entire cluster
	multicastFn = func(message *proto.Message) {
		cluster.pushMessage(message)
	}

	// Start the main run loops
	cluster.runSequence(1)

	// Wait until the main run loops finish
	cluster.stop()

	// Make sure the nodes switched to the new round
	assert.True(t, cluster.areAllNodesOnRound(1))

	// Make sure the inserted blocks match what node 1 proposed
	for _, block := range insertedBlocks {
		assert.True(t, bytes.Equal(block, proposals[1]))
	}
}

//// TestConsensus_Persistence verifies the persistence problem
//// outlined in the following analysis paper:
//// https://arxiv.org/pdf/1901.07160.pdf
//func TestConsensus_Persistence(t *testing.T) {
//	// TODO implement
//}
//
//// TestConsensus_Liveness verifies the liveness problem
//// outlined in the following analysis paper:
//// https://arxiv.org/pdf/1901.07160.pdf
//func TestConsensus_Liveness(t *testing.T) {
//	// TODO implement
//}
