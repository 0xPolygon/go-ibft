package core

import (
	"bytes"
	"context"
	"sync"
	"testing"

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

// propertyTestEvent contains randomly-generated data for rapid testing
type propertyTestEvent struct {
	// nodes is the total number of nodes
	nodes uint64

	// byzantineNodes is the total number of byzantine nodes
	byzantineNodes uint64

	// silentByzantineNodes is the number of byzantine nodes
	// that are going to be silent, i.e. do not respond
	silentByzantineNodes uint64

	// badByzantineNodes is the number of byzantine nodes
	// that are going to send bad messages
	badByzantineNodes uint64

	// desiredHeight is the desired height number
	desiredHeight uint64
}

// generatePropertyTestEvent generates propertyTestEvent model
func generatePropertyTestEvent(t *rapid.T) *propertyTestEvent {
	var (
		numNodes             = rapid.Uint64Range(4, 15).Draw(t, "number of cluster nodes")
		numByzantineNodes    = rapid.Uint64Range(0, maxFaulty(numNodes)).Draw(t, "number of byzantine nodes")
		silentByzantineNodes = rapid.Uint64Range(0, numByzantineNodes).Draw(t, "number of silent byzantine nodes")
		desiredHeight        = rapid.Uint64Range(10, 20).Draw(t, "minimum height to be reached")
	)

	return &propertyTestEvent{
		nodes:                numNodes,
		byzantineNodes:       numByzantineNodes,
		silentByzantineNodes: silentByzantineNodes,
		badByzantineNodes:    numByzantineNodes - silentByzantineNodes,
		desiredHeight:        desiredHeight,
	}
}

// TestProperty is a property-based test
// that assures the cluster can handle rounds properly in any cases.
func TestProperty(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			testEvent         = generatePropertyTestEvent(t)
			currentQuorum     = quorum(testEvent.nodes)
			nodes             = generateNodeAddresses(testEvent.nodes)
			insertedProposals = newMockInsertedProposals(testEvent.nodes)
		)

		// commonTransportCallback is the common method modification
		// required for Transport, for all nodes
		commonTransportCallback := func(transport *mockTransport, nodeIndex int) {
			transport.multicastFn = func(message *proto.Message) {
				// If node is silent, don't send a message
				if uint64(nodeIndex) >= testEvent.byzantineNodes &&
					uint64(nodeIndex) < testEvent.silentByzantineNodes {
					return
				}

				multicastFn(message)
			}
		}

		// commonBackendCallback is the common method modification required
		// for the Backend, for all nodes
		commonBackendCallback := func(backend *mockBackend, nodeIndex int) {
			// Use a bad message if the current node is a bad byzantine one
			message := correctRoundMessage
			if uint64(nodeIndex) < testEvent.byzantineNodes {
				message = badRoundMessage
			}

			// Make sure the quorum function is Quorum optimal
			backend.hasQuorumFn = commonHasQuorumFn(testEvent.nodes)

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
			backend.buildProposalFn = func(_ *proto.View) []byte {
				return message.proposal
			}
		}

		// Create default cluster for rapid tests
		cluster := newMockCluster(testEvent.nodes, commonBackendCallback, nil, commonTransportCallback)

		// Set the multicast callback to relay the message
		// to the entire cluster
		multicastFn = cluster.pushMessage

		// Minimum one round is required
		minRounds := uint64(1)
		if testEvent.byzantineNodes > minRounds {
			minRounds = testEvent.byzantineNodes
		}

		// Create context timeout based on the bad nodes number
		ctxTimeout := getRoundTimeout(testRoundTimeout, testRoundTimeout, minRounds+1)

		// Run the sequence up until a certain height
		for height := uint64(0); height < testEvent.desiredHeight; height++ {
			// Start the main run loops
			cluster.runSequence(height)

			if testEvent.byzantineNodes == 0 {
				// Wait until all nodes propose messages
				cluster.awaitCompletion()
			} else {
				// Wait until Quorum nodes finish their run loop
				ctx, cancelFn := context.WithTimeout(context.Background(), ctxTimeout)
				err := cluster.awaitNCompletions(ctx, int64(currentQuorum))
				assert.NoError(t, err, "unable to wait for nodes to complete on height %d", height)
				cancelFn()
			}

			// Shutdown the remaining nodes that might be hanging
			cluster.forceShutdown()
		}

		// Make sure proposals map is not empty
		require.Len(t, insertedProposals.proposals, int(testEvent.nodes))

		// Make sure that the inserted proposal is valid for each height
		for i, proposalMap := range insertedProposals.proposals {
			if i < int(testEvent.byzantineNodes) {
				// Proposals map must be empty when a byzantine node is proposer
				assert.Empty(t, proposalMap)
			} else {
				// Make sure the node has proposals
				assert.NotEmpty(t, proposalMap)

				// Check values
				for _, insertedProposal := range proposalMap {
					assert.Equal(t, correctRoundMessage.proposal, insertedProposal)
				}
			}
		}
	})
}
