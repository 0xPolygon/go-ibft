package core

import (
	"bytes"
	"testing"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// TestProperty_AllHonestNodes is a property-based test
// that assures the cluster can reach consensus on any
// arbitrary number of valid nodes
func TestProperty_AllHonestNodes(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			proposal      = []byte("proposal")
			proposalHash  = []byte("proposal hash")
			committedSeal = []byte("seal")

			numNodes      = rapid.Uint64Range(4, 30).Draw(t, "number of cluster nodes")
			desiredHeight = rapid.Uint64Range(10, 20).Draw(t, "minimum height to be reached")

			nodes            = generateNodeAddresses(numNodes)
			insertedBlocks   = make([]map[uint64][]byte, numNodes)
			currentProposals = make([]uint64, numNodes)
		)

		// Initialize the block insertion map, used for lookups
		for i := uint64(0); i < numNodes; i++ {
			insertedBlocks[i] = make(map[uint64][]byte)
		}

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
				return numNodes
			}

			// Make sure the node ID is properly relayed
			backend.idFn = func() []byte {
				return nodes[nodeIndex]
			}

			// Make sure the only proposer is picked using Round Robin
			backend.isProposerFn = func(from []byte, height uint64, _ uint64) bool {
				return bytes.Equal(from, nodes[height%numNodes])
			}

			// Make sure the proposal is valid if it matches what node 0 proposed
			backend.isValidBlockFn = func(newProposal []byte) bool {
				return bytes.Equal(newProposal, proposal)
			}

			// Make sure the proposal hash matches
			backend.isValidProposalHashFn = func(p []byte, ph []byte) bool {
				return bytes.Equal(p, proposal) && bytes.Equal(ph, proposalHash)
			}

			// Make sure the preprepare message is built correctly
			backend.buildPrePrepareMessageFn = func(
				proposal []byte,
				certificate *proto.RoundChangeCertificate,
				view *proto.View,
			) *proto.Message {
				return buildBasicPreprepareMessage(
					proposal,
					proposalHash,
					certificate,
					nodes[nodeIndex],
					view)
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
			backend.buildRoundChangeMessageFn = func(
				proposal []byte,
				certificate *proto.PreparedCertificate,
				view *proto.View,
			) *proto.Message {
				return buildBasicRoundChangeMessage(proposal, certificate, view, nodes[nodeIndex])
			}

			// Make sure the inserted proposal is noted
			backend.insertBlockFn = func(proposal []byte, _ []*messages.CommittedSeal) {
				insertedBlocks[nodeIndex][currentProposals[nodeIndex]] = proposal
				currentProposals[nodeIndex]++
			}

			// Make sure the proposal can be built
			backend.buildProposalFn = func(u uint64) []byte {
				return proposal
			}
		}

		// Initialize the backend and transport callbacks for
		// each node in the arbitrary cluster
		backendCallbackMap := make(map[int]backendConfigCallback)
		transportCallbackMap := make(map[int]transportConfigCallback)

		for i := 0; i < int(numNodes); i++ {
			i := i
			backendCallbackMap[i] = func(backend *mockBackend) {
				commonBackendCallback(backend, i)
			}

			transportCallbackMap[i] = commonTransportCallback
		}

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

		// Run the sequence up until a certain height
		cluster.runSequenceUntilHeight(desiredHeight)

		// Make sure that the inserted block is valid for each height
		for _, blockMap := range insertedBlocks {
			// Make sure the node has the adequate number of inserted blocks
			assert.Len(t, blockMap, int(desiredHeight))

			for _, block := range blockMap {
				assert.True(t, bytes.Equal(proposal, block))
			}
		}
	})
}
