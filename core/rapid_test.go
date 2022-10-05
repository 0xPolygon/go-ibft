package core

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

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
		for height := uint64(0); height < desiredHeight; height++ {
			// Start the main run loops
			cluster.runSequence(height)

			// Wait until the main run loops finish
			cluster.awaitCompletion()
		}

		// Make sure that the inserted proposal is valid for each height
		for _, proposalMap := range insertedBlocks {
			// Make sure the node has the adequate number of inserted proposals
			assert.Len(t, proposalMap, int(desiredHeight))

			for _, insertedProposal := range proposalMap {
				assert.True(t, bytes.Equal(proposal, insertedProposal))
			}
		}
	})
}

// TestProperty_MajorityHonestNodes is a property-based test
// that assures the cluster can reach consensus on any
// arbitrary number of valid nodes and byzantine nodes
func TestProperty_MajorityHonestNodes(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			proposal      = []byte("proposal")
			proposalHash  = []byte("proposal hash")
			committedSeal = []byte("seal")

			numNodes          = rapid.Uint64Range(4, 30).Draw(t, "number of cluster nodes")
			numByzantineNodes = rapid.Uint64Range(1, maxFaulty(numNodes)).Draw(t, "number of byzantine nodes")
			desiredHeight     = rapid.Uint64Range(1, 5).Draw(t, "minimum height to be reached")

			nodes            = generateNodeAddresses(numNodes)
			insertedBlocks   = make([]map[uint64][]byte, numNodes)
			currentProposals = make([]uint64, numNodes)
		)
		// Initialize the block insertion map, used for lookups
		for i := uint64(0); i < numNodes; i++ {
			insertedBlocks[i] = make(map[uint64][]byte)
		}

		// Initialize the byzantine nodes
		gen := rapid.SampledFrom(nodes)
		byzantineNodes := make(map[string]struct{})
		for i := 0; i < int(numByzantineNodes); i++ {
			byzantineNodes[fmt.Sprintf("%s", gen.Example(i))] = struct{}{}
		}

		isByzantineNode := func(from []byte) bool {
			_, exists := byzantineNodes[fmt.Sprintf("%s", from)]

			return exists
		}

		// commonTransportCallback is the common method modification
		// required for Transport, for all nodes
		commonTransportCallback := func(transport *mockTransport) {
			transport.multicastFn = func(message *proto.Message) {
				if isByzantineNode(message.From) {
					// If the node is byzantine, mock
					// not sending out the message
					return
				}

				multicastFn(message)
			}
		}

		// commonBackendCallback is the common method modification required
		// for the Backend, for all nodes
		commonBackendCallback := func(backend *mockBackend, nodeIndex int) {
			// Make sure the quorum function is Quorum optimal
			backend.quorumFn = func(_ uint64) uint64 {
				return quorumOptimal(numNodes)
			}

			// Make sure the allowed faulty nodes function is accurate
			backend.maximumFaultyNodesFn = func() uint64 {
				return maxFaulty(numNodes)
			}

			// Make sure the node ID is properly relayed
			backend.idFn = func() []byte {
				return nodes[nodeIndex]
			}

			// Make sure the only proposer is picked using Round Robin
			backend.isProposerFn = func(from []byte, height uint64, round uint64) bool {
				return bytes.Equal(
					from,
					nodes[int(height+round)%len(nodes)],
				)
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
				if isByzantineNode(nodes[nodeIndex]) {
					return
				}

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

		// Set a small timeout, because of situations
		// where the byzantine node is the proposer
		cluster.setBaseTimeout(time.Second * 2)

		// Set the multicast callback to relay the message
		// to the entire cluster
		multicastFn = func(message *proto.Message) {
			cluster.pushMessage(message)
		}

		// Run the sequence up until a certain height
		for height := uint64(0); height < desiredHeight; height++ {
			// Start the main run loops
			cluster.runSequence(height)

			// Wait until Quorum nodes finish their run loop
			ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*5)
			if err := cluster.awaitNCompletions(ctx, int64(quorumOptimal(numNodes))); err != nil {
				t.Fatalf(
					fmt.Sprintf(
						"unable to wait for nodes to complete, %v",
						err,
					),
				)
			}

			// Shutdown the remaining nodes that might be hanging
			cluster.forceShutdown()
			cancelFn()
		}

		// Make sure that the inserted proposal is valid for each height
		for _, proposalMap := range insertedBlocks {
			for _, insertedProposal := range proposalMap {
				assert.True(t, bytes.Equal(proposal, insertedProposal))
			}
		}
	})
}
