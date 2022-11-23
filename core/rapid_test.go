package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	m.proposals[nodeIndex][m.currentProposals[nodeIndex]] = proposal
	m.currentProposals[nodeIndex]++
	m.Unlock()
}

// getProposer returns proposer index
func getProposer(height, round, nodes uint64) uint64 {
	return (height + round) % nodes
}

// propertyTestEvent is the behaviour setup per specific round
type propertyTestEvent struct {
	// silentByzantineNodes is the number of byzantine nodes
	// that are going to be silent, i.e. do not respond
	silentByzantineNodes uint64

	// badByzantineNodes is the number of byzantine nodes
	// that are going to send bad messages
	badByzantineNodes uint64
}

func (e propertyTestEvent) badNodes() uint64 {
	return e.silentByzantineNodes + e.badByzantineNodes
}

func (e propertyTestEvent) isSilent(nodeIndex int) bool {
	return uint64(nodeIndex) < e.silentByzantineNodes
}

// getMessage returns bad message for byzantine bad node,
// correct message for non-byzantine nodes, and nil for silent nodes
func (e propertyTestEvent) getMessage(nodeIndex int) *roundMessage {
	message := correctRoundMessage
	if uint64(nodeIndex) < e.badNodes() {
		message = badRoundMessage
	}
	return &message
}

// propertyTestSetup contains randomly-generated data for rapid testing
type propertyTestSetup struct {
	sync.Mutex

	// nodes is the total number of nodes
	nodes uint64

	// desiredHeight is the desired height number
	desiredHeight uint64

	// events is the mapping between the current height and its rounds
	events [][]propertyTestEvent

	currentHeight map[int]uint64
	currentRound  map[int]uint64
}

func (s *propertyTestSetup) setRound(nodeIndex int, round uint64) {
	s.Lock()
	s.currentRound[nodeIndex] = round
	s.Unlock()
}

func (s *propertyTestSetup) incHeight() {
	s.Lock()
	for nodeIndex := 0; uint64(nodeIndex) < s.nodes; nodeIndex++ {
		s.currentHeight[nodeIndex]++
		s.currentRound[nodeIndex] = 0
	}
	s.Unlock()
}

func (s *propertyTestSetup) getEvent(nodeIndex int) propertyTestEvent {
	s.Lock()

	height := int(s.currentHeight[nodeIndex])
	roundNumber := int(s.currentRound[nodeIndex])
	var round propertyTestEvent
	if roundNumber >= len(s.events[height]) {
		round = s.events[height][len(s.events[height])-1]
	} else {
		round = s.events[height][roundNumber]
	}

	s.Unlock()

	return round
}

func (s *propertyTestSetup) lastRound(height uint64) propertyTestEvent {
	return s.events[height][len(s.events[height])-1]
}

func (s *propertyTestSetup) lastRoundQuorum(height uint64) int64 {
	lastRound := s.lastRound(height)
	return int64(s.nodes - lastRound.badNodes())
}

// generatePropertyTestEvent generates propertyTestEvent model
func generatePropertyTestEvent(t *rapid.T) *propertyTestSetup {
	var (
		numNodes      = rapid.Uint64Range(4, 15).Draw(t, "number of cluster nodes")
		desiredHeight = rapid.Uint64Range(1, 5).Draw(t, "minimum height to be reached")
		maxBadNodes   = maxFaulty(numNodes)
	)

	setup := &propertyTestSetup{
		nodes:         numNodes,
		desiredHeight: desiredHeight,
		events:        make([][]propertyTestEvent, desiredHeight),
		currentHeight: map[int]uint64{},
		currentRound:  map[int]uint64{},
	}

	for height := uint64(0); height < desiredHeight; height++ {
		var round uint64
		for {
			numByzantineNodes := rapid.
				Uint64Range(0, maxBadNodes).
				Draw(t, fmt.Sprintf("number of byzantine nodes for height %d on round %d", height, round))
			silentByzantineNodes := rapid.
				Uint64Range(0, numByzantineNodes).
				Draw(t, fmt.Sprintf("number of silent byzantine nodes for height %d on round %d", height, round))
			proposerIdx := getProposer(height, round, numNodes)

			setup.events[height] = append(setup.events[height], propertyTestEvent{
				silentByzantineNodes: silentByzantineNodes,
				badByzantineNodes:    numByzantineNodes - silentByzantineNodes,
			})

			if proposerIdx >= numByzantineNodes {
				break
			}

			round++
		}
	}

	fmt.Printf("nodes: %d, height: %d, faulty: %d\n", numNodes, desiredHeight, maxFaulty(numNodes))
	for height, rounds := range setup.events {
		for round, data := range rounds {
			fmt.Println(" ",
				"height:", height,
				"round:", round,
				"proposer:", getProposer(uint64(height), uint64(round), setup.nodes),
				"byz:", data.silentByzantineNodes+data.badByzantineNodes,
			)
		}
		fmt.Println()
	}

	return setup
}

// TestProperty is a property-based test
// that assures the cluster can handle rounds properly in any cases.
func TestProperty(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		var multicastFn func(message *proto.Message)

		var (
			setup             = generatePropertyTestEvent(t)
			nodes             = generateNodeAddresses(setup.nodes)
			insertedProposals = newMockInsertedProposals(setup.nodes)
		)

		// commonTransportCallback is the common method modification
		// required for Transport, for all nodes
		commonTransportCallback := func(transport *mockTransport, nodeIndex int) {
			transport.multicastFn = func(message *proto.Message) {
				if message.Type == proto.MessageType_ROUND_CHANGE {
					setup.setRound(nodeIndex, message.View.Round)
				}

				// If node is silent, don't send a message
				if setup.getEvent(nodeIndex).isSilent(nodeIndex) {
					return
				}

				multicastFn(message)
			}
		}

		// commonBackendCallback is the common method modification required
		// for the Backend, for all nodes
		commonBackendCallback := func(backend *mockBackend, nodeIndex int) {
			// Make sure the quorum function is Quorum optimal
			backend.hasQuorumFn = commonHasQuorumFn(setup.nodes)

			// Make sure the node ID is properly relayed
			backend.idFn = func() []byte {
				return nodes[nodeIndex]
			}

			// Make sure the only proposer is picked using Round Robin
			backend.isProposerFn = func(from []byte, height, round uint64) bool {
				return bytes.Equal(
					from,
					nodes[getProposer(height, round, setup.nodes)],
				)
			}

			// Make sure the proposal is valid if it matches what node 0 proposed
			backend.isValidBlockFn = func(newProposal []byte) bool {
				message := setup.getEvent(nodeIndex).getMessage(nodeIndex)
				return bytes.Equal(newProposal, message.proposal)
			}

			// Make sure the proposal hash matches
			backend.isValidProposalHashFn = func(p []byte, ph []byte) bool {
				message := setup.getEvent(nodeIndex).getMessage(nodeIndex)
				return bytes.Equal(p, message.proposal) && bytes.Equal(ph, message.hash)
			}

			// Make sure the preprepare message is built correctly
			backend.buildPrePrepareMessageFn = func(
				proposal []byte,
				certificate *proto.RoundChangeCertificate,
				view *proto.View,
			) *proto.Message {
				message := setup.getEvent(nodeIndex).getMessage(nodeIndex)
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
				message := setup.getEvent(nodeIndex).getMessage(nodeIndex)
				return buildBasicPrepareMessage(message.hash, nodes[nodeIndex], view)
			}

			// Make sure the commit message is built correctly
			backend.buildCommitMessageFn = func(proposal []byte, view *proto.View) *proto.Message {
				message := setup.getEvent(nodeIndex).getMessage(nodeIndex)
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
			backend.buildProposalFn = func(view *proto.View) []byte {
				message := setup.getEvent(nodeIndex).getMessage(nodeIndex)
				return message.proposal
			}
		}

		// Create default cluster for rapid tests
		cluster := newMockCluster(
			setup.nodes,
			commonBackendCallback,
			defaultLoggerCallback,
			commonTransportCallback,
		)

		// Set the multicast callback to relay the message
		// to the entire cluster
		multicastFn = cluster.pushMessage

		// Run the sequence up until a certain height
		for height := uint64(0); height < setup.desiredHeight; height++ {
			// Create context timeout based on the bad nodes number
			rounds := uint64(len(setup.events[height]))
			ctxTimeout := getRoundTimeout(testRoundTimeout, testRoundTimeout, rounds*2)

			// Get number of valid nodes in the last round per height
			currentQuorum := setup.lastRoundQuorum(height)

			// Start the main run loops
			cluster.runSequence(height)

			if uint64(currentQuorum) == setup.nodes {
				// Wait until all nodes finish their run loop
				cluster.awaitCompletion()
			} else {
				// Wait until Quorum nodes finish their run loop
				ctx, cancelFn := context.WithTimeout(context.Background(), ctxTimeout)
				err := cluster.awaitNCompletions(ctx, currentQuorum)
				assert.NoError(t, err, "unable to wait for nodes to complete on height %d", height)
				cancelFn()
			}

			// Shutdown the remaining nodes that might be hanging
			cluster.forceShutdown()

			setup.incHeight()
		}

		// Make sure proposals map is not empty
		require.Len(t, insertedProposals.proposals, int(setup.nodes))

		// Make sure that the inserted proposal is valid for each height
		nodesToBlocks := make(map[int]int)
		for height := range setup.events {
			lastRound := setup.lastRound(uint64(height))
			badNodes := lastRound.badNodes() - 1

			for nodeID := 0; nodeID < int(setup.nodes); nodeID++ {
				if _, ok := nodesToBlocks[nodeID]; !ok {
					nodesToBlocks[nodeID] = 0
				}

				if nodeID > int(badNodes) {
					nodesToBlocks[nodeID]++
				}
			}
		}

		proposalsRaw, _ := json.MarshalIndent(insertedProposals.proposals, "", "  ")
		nodesToBlocksRaw, _ := json.MarshalIndent(nodesToBlocks, "", "  ")
		fmt.Println(string(proposalsRaw))
		fmt.Println(string(nodesToBlocksRaw))

		for nodeID, proposalMap := range insertedProposals.proposals {
			assert.Len(t, proposalMap, nodesToBlocks[nodeID])

			for _, val := range proposalMap {
				assert.Equal(t, correctRoundMessage.proposal, val)
			}
		}

		fmt.Println("---------------------------------------------------------")
	})
}
