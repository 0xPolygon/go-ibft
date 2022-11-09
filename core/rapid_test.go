package core

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
)

// roundMessage contains message data within consensus round
type roundMessage struct {
	proposal []byte
	seal     []byte
	hash     []byte
}

// mockInsertedProposals keeps track of inserted proposals for a cluster
// of nodes
type mockInsertedProposals struct {
	sync.Mutex

	proposals        []map[uint64][]byte // for each node, map the height -> proposal
	currentProposals []uint64            // for each node, save the current proposal height
}

// newMockInsertedProposals creates a new proposal insertion tracker
func newMockInsertedProposals(numNodes uint64) *mockInsertedProposals {
	m := &mockInsertedProposals{
		proposals:        make([]map[uint64][]byte, numNodes),
		currentProposals: make([]uint64, numNodes),
	}

	// Initialize the proposal insertion map, used for lookups
	for i := uint64(0); i < numNodes; i++ {
		m.proposals[i] = make(map[uint64][]byte)
	}

	return m
}

// insertProposal inserts a new proposal for the specified node [Thread safe]
func (m *mockInsertedProposals) insertProposal(
	nodeIndex int,
	proposal []byte,
) {
	m.Lock()
	defer m.Unlock()

	m.proposals[nodeIndex][m.currentProposals[nodeIndex]] = proposal
	m.currentProposals[nodeIndex]++
}

// TestProperty_AllHonestNodes is a property-based test
// that assures the cluster can reach consensus on any
// arbitrary number of valid nodes
func TestProperty_AllHonestNodes(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			message = roundMessage{
				proposal: []byte("proposal"),
				hash:     []byte("proposal hash"),
				seal:     []byte("seal"),
			}

			numNodes      = rapid.Uint64Range(4, 30).Draw(t, "number of cluster nodes")
			desiredHeight = rapid.Uint64Range(10, 20).Draw(t, "minimum height to be reached")

			nodes             = generateNodeAddresses(numNodes)
			insertedProposals = newMockInsertedProposals(numNodes)
		)
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
				return bytes.Equal(newProposal, message.proposal)
			}

			// Make sure the proposal hash matches
			backend.isValidProposalHashFn = func(p []byte, ph []byte) bool {
				return bytes.Equal(p, message.proposal) && bytes.Equal(ph, message.hash)
			}

			// Make sure the preprepare message is built correctly
			backend.buildPrePrepareMessageFn = func(
				proposal []byte,
				certificate *proto.RoundChangeCertificate,
				view *proto.View,
			) *proto.Message {
				return buildBasicPreprepareMessage(
					proposal,
					message.hash,
					certificate,
					nodes[nodeIndex],
					view)
			}

			// Make sure the prepare message is built correctly
			backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				return buildBasicPrepareMessage(message.hash, nodes[nodeIndex], view)
			}

			// Make sure the commit message is built correctly
			backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				return buildBasicCommitMessage(message.hash, message.seal, nodes[nodeIndex], view)
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
				insertedProposals.insertProposal(nodeIndex, proposal)
			}

			// Make sure the proposal can be built
			backend.buildProposalFn = func(u uint64) []byte {
				return message.proposal
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
		for _, proposalMap := range insertedProposals.proposals {
			// Make sure the node has the adequate number of inserted proposals
			assert.Len(t, proposalMap, int(desiredHeight))

			for _, insertedProposal := range proposalMap {
				assert.Equal(t, message.proposal, insertedProposal)
			}
		}
	})
}

// getByzantineNodes returns a random subset of
// byzantine nodes
func getByzantineNodes(
	numNodes uint64,
	set [][]byte,
) map[string]struct{} {
	gen := rapid.SampledFrom(set)
	byzantineNodes := make(map[string]struct{})

	for i := 0; i < int(numNodes); i++ {
		byzantineNodes[string(gen.Example(i))] = struct{}{}
	}

	return byzantineNodes
}

// TestProperty_MajorityHonestNodes is a property-based test
// that assures the cluster can reach consensus on any
// arbitrary number of valid nodes and byzantine nodes
func TestProperty_MajorityHonestNodes(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			message = roundMessage{
				proposal: []byte("proposal"),
				hash:     []byte("proposal hash"),
				seal:     []byte("seal"),
			}

			numNodes          = rapid.Uint64Range(4, 30).Draw(t, "number of cluster nodes")
			numByzantineNodes = rapid.Uint64Range(1, maxFaulty(numNodes)).Draw(t, "number of byzantine nodes")
			desiredHeight     = rapid.Uint64Range(1, 5).Draw(t, "minimum height to be reached")

			nodes             = generateNodeAddresses(numNodes)
			insertedProposals = newMockInsertedProposals(numNodes)
		)
		// Initialize the byzantine nodes
		byzantineNodes := getByzantineNodes(
			numByzantineNodes,
			nodes,
		)

		isByzantineNode := func(from []byte) bool {
			_, exists := byzantineNodes[string(from)]

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
				return quorum(numNodes)
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
				return bytes.Equal(newProposal, message.proposal)
			}

			// Make sure the proposal hash matches
			backend.isValidProposalHashFn = func(p []byte, ph []byte) bool {
				return bytes.Equal(p, message.proposal) && bytes.Equal(ph, message.hash)
			}

			// Make sure the preprepare message is built correctly
			backend.buildPrePrepareMessageFn = func(
				proposal []byte,
				certificate *proto.RoundChangeCertificate,
				view *proto.View,
			) *proto.Message {
				return buildBasicPreprepareMessage(
					proposal,
					message.hash,
					certificate,
					nodes[nodeIndex],
					view,
				)
			}

			// Make sure the prepare message is built correctly
			backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				return buildBasicPrepareMessage(message.hash, nodes[nodeIndex], view)
			}

			// Make sure the commit message is built correctly
			backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				return buildBasicCommitMessage(message.hash, message.seal, nodes[nodeIndex], view)
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
				insertedProposals.insertProposal(nodeIndex, proposal)
			}

			// Make sure the proposal can be built
			backend.buildProposalFn = func(u uint64) []byte {
				return message.proposal
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
			err := cluster.awaitNCompletions(ctx, int64(quorum(numNodes)))
			require.NoError(t, err, "unable to wait for nodes to complete")

			// Shutdown the remaining nodes that might be hanging
			cluster.forceShutdown()
			cancelFn()
		}

		// Make sure that the inserted proposal is valid for each height
		for _, proposalMap := range insertedProposals.proposals {
			for _, insertedProposal := range proposalMap {
				assert.Equal(t, message.proposal, insertedProposal)
			}
		}
	})
}

// TestProperty_MajorityHonestNodes_BadProposal is a property-based test
// that assures the cluster can reach consensus on any
// arbitrary number of valid nodes and byzantine nodes
func TestProperty_MajorityHonestNodes_BadProposal(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			correctMessage = roundMessage{
				proposal: []byte("proposal"),
				hash:     []byte("proposal hash"),
				seal:     []byte("seal"),
			}

			badMessage = roundMessage{
				proposal: []byte("bad proposal"),
				hash:     []byte("bad proposal hash"),
				seal:     []byte("bad seal"),
			}

			numNodes          = rapid.Uint64Range(4, 30).Draw(t, "number of cluster nodes")
			numByzantineNodes = rapid.Uint64Range(1, maxFaulty(numNodes)).Draw(t, "number of byzantine nodes")
			desiredHeight     = rapid.Uint64Range(1, 5).Draw(t, "minimum height to be reached")

			nodes             = generateNodeAddresses(numNodes)
			insertedProposals = newMockInsertedProposals(numNodes)
		)

		commonLoggerCallback := func(logger *mockLogger) {
			logger.infoFn = func(s string, i ...interface{}) {
				fmt.Println(append([]interface{}{s}, i...)...)
				t.Logf(s, i...)
			}

			logger.errorFn = func(s string, i ...interface{}) {
				fmt.Println(append([]interface{}{s}, i...)...)
				t.Errorf(s, i...)
			}

			logger.debugFn = func(s string, i ...interface{}) {
				fmt.Println(append([]interface{}{s}, i...)...)
				t.Logf(s, i...)
			}
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
			message := correctMessage
			if uint64(nodeIndex) < numByzantineNodes {
				message = badMessage
			}

			// Make sure the quorum function is Quorum optimal
			backend.quorumFn = func(_ uint64) uint64 {
				return quorum(numNodes)
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
				nodes := nodes[numByzantineNodes:]
				return bytes.Equal(
					from,
					nodes[int(height+round)%len(nodes)],
				)
			}

			// Make sure the proposal is valid if it matches what node 0 proposed
			backend.isValidBlockFn = func(newProposal []byte) bool {
				return bytes.Equal(newProposal, message.proposal)
			}

			// Make sure the proposal hash matches
			backend.isValidProposalHashFn = func(p []byte, ph []byte) bool {
				return bytes.Equal(p, message.proposal) && bytes.Equal(ph, message.hash)
			}

			// Make sure the preprepare message is built correctly
			backend.buildPrePrepareMessageFn = func(
				proposal []byte,
				certificate *proto.RoundChangeCertificate,
				view *proto.View,
			) *proto.Message {
				return buildBasicPreprepareMessage(
					proposal,
					message.hash,
					certificate,
					nodes[nodeIndex],
					view,
				)
			}

			// Make sure the prepare message is built correctly
			backend.buildPrepareMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				return buildBasicPrepareMessage(message.hash, nodes[nodeIndex], view)
			}

			// Make sure the commit message is built correctly
			backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				return buildBasicCommitMessage(message.hash, message.seal, nodes[nodeIndex], view)
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
				insertedProposals.insertProposal(nodeIndex, proposal)
			}

			// Make sure the proposal can be built
			backend.buildProposalFn = func(u uint64) []byte {
				return message.proposal
			}
		}

		// Initialize the backend and transport callbacks for
		// each node in the arbitrary cluster
		loggerCallbackMap := make(map[int]loggerConfigCallback)
		backendCallbackMap := make(map[int]backendConfigCallback)
		transportCallbackMap := make(map[int]transportConfigCallback)

		for i := 0; i < int(numNodes); i++ {
			i := i
			backendCallbackMap[i] = func(backend *mockBackend) {
				commonBackendCallback(backend, i)
			}

			transportCallbackMap[i] = commonTransportCallback

			loggerCallbackMap[i] = commonLoggerCallback
		}

		// Create the mock cluster
		cluster := newMockCluster(
			numNodes,
			backendCallbackMap,
			loggerCallbackMap,
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
			err := cluster.awaitNCompletions(ctx, int64(quorum(numNodes)))
			require.NoError(t, err, "unable to wait for nodes to complete")

			// Shutdown the remaining nodes that might be hanging
			cluster.forceShutdown()
			cancelFn()
		}

		// Make sure that the inserted proposal is valid for each height
		for _, proposalMap := range insertedProposals.proposals {
			for _, insertedProposal := range proposalMap {
				assert.Equal(t, correctMessage.proposal, insertedProposal)
			}
		}
	})
}
