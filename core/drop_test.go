package core

import (
	"bytes"
	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDropAllAndRecover(t *testing.T) {
	t.Parallel()

	var (
		numNodes       = uint64(6)
		insertedBlocks = make([][]byte, numNodes)
	)

	cluster := newCluster(
		numNodes,
		func(c *cluster) {
			for nodeIndex, node := range c.nodes {
				i := nodeIndex
				currentNode := node
				node.core = NewIBFT(
					mockLogger{},
					&mockBackend{
						isValidBlockFn:         isValidProposal,
						isValidProposalHashFn:  isValidProposalHash,
						isValidSenderFn:        nil,
						isValidCommittedSealFn: nil,
						isProposerFn:           c.isProposer,

						idFn: node.addr,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,

						insertBlockFn: func(proposal []byte, _ []*messages.CommittedSeal) {
							insertedBlocks[i] = proposal
						},
						hasQuorumFn: c.hasQuorumFn,
					},
					&mockTransport{multicastFn: func(message *proto.Message) {
						if currentNode.offline {
							return
						}

						c.gossip(message)
					}},
				)
			}
		},
	)

	// Progress the chain to claim it works ok by default
	err := cluster.progressToHeight(5*time.Second, 1)
	assert.NoError(t, err, "unable to reach height: %w", err)
	assert.Equal(t, uint64(1), cluster.latestHeight)
	assertValidInsertedBlocks(insertedBlocks, t) // Make sure the inserted blocks are valid

	insertedBlocks = make([][]byte, numNodes) // Purge

	// Stop all nodes and make sure no blocks are written
	cluster.stopN(len(cluster.nodes))
	cluster.progressToHeight(5*time.Second, 2)
	assertNInsertedBlocks(0, insertedBlocks, t)

	// Start all and expect valid blocks to be written again
	cluster.startN(len(cluster.nodes))
	cluster.progressToHeight(5*time.Second, 10)
	assertValidInsertedBlocks(insertedBlocks, t) // Make sure the inserted blocks are valid
}

func assertNInsertedBlocks(n int, blocks [][]byte, t *testing.T) {
	t.Helper()

	writtenBlocks := 0
	for _, block := range blocks {
		if !bytes.Equal(block, nil) {
			writtenBlocks++
		}
	}

	assert.True(t, n == writtenBlocks)
}

func assertValidInsertedBlocks(blocks [][]byte, t *testing.T) {
	t.Helper()

	for _, block := range blocks {
		assert.True(t, bytes.Equal(block, validProposal))
	}
}

func TestMaxFaultyDroppingMessages(t *testing.T) {
	t.Parallel()

	cluster := newCluster(
		6,
		func(c *cluster) {
			for _, node := range c.nodes {
				currentNode := node
				node.core = NewIBFT(
					mockLogger{},
					&mockBackend{
						isValidBlockFn:         isValidProposal,
						isValidProposalHashFn:  isValidProposalHash,
						isValidSenderFn:        nil,
						isValidCommittedSealFn: nil,
						isProposerFn:           c.isProposer,

						idFn: node.addr,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,

						insertBlockFn: nil,
						hasQuorumFn:   c.hasQuorumFn,
					},
					&mockTransport{multicastFn: func(message *proto.Message) {
						if currentNode.faulty && rand.Intn(100) < 50 {
							return
						}

						c.gossip(message)
					}},
				)
			}
		},
	)

	cluster.makeNFaulty(int(cluster.maxFaulty()))
	assert.NoError(t, cluster.progressToHeight(40*time.Second, 5))
	assert.Equal(t, uint64(5), cluster.latestHeight)
}

func TestAllFailAndGraduallyRecover(t *testing.T) {
	t.Parallel()

	var (
		numNodes       = uint64(6)
		insertedBlocks = make([][]byte, numNodes)
	)

	cluster := newCluster(
		numNodes,
		func(c *cluster) {
			for nodeIndex, node := range c.nodes {
				nodeIndex := nodeIndex
				currentNode := node
				node.core = NewIBFT(
					mockLogger{},
					&mockBackend{
						isValidBlockFn:         isValidProposal,
						isValidProposalHashFn:  isValidProposalHash,
						isValidSenderFn:        nil,
						isValidCommittedSealFn: nil,
						isProposerFn:           c.isProposer,

						idFn: node.addr,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,

						insertBlockFn: func(proposal []byte, _ []*messages.CommittedSeal) {
							insertedBlocks[nodeIndex] = proposal
						},
						hasQuorumFn: c.hasQuorumFn,
					},
					&mockTransport{multicastFn: func(msg *proto.Message) {
						if !currentNode.offline {
							for _, node := range c.nodes {
								node.core.AddMessage(msg)
							}
						}
					}},
				)
			}
		},
	)

	// Start the main run loops
	cluster.runGradualSequence(1, 10*time.Second)

	// Wait until the main run loops finish
	cluster.wg.Wait()

	// Make sure the inserted blocks match what node 0 proposed
	for _, block := range insertedBlocks {
		assert.True(t, bytes.Equal(block, validProposal))
	}
}

/*
Scenario:
1. Cluster can reach height 5
2. Stop MaxFaulty+1 nodes
3. Cluster cannot reach height 10
4. Start MaxFaulty+1 nodes
5. Cluster can reach height 10
*/
func TestDropMaxFaultyPlusOne(t *testing.T) {
	t.Parallel()

	cluster := newCluster(
		6,
		func(c *cluster) {
			for _, node := range c.nodes {
				node.core = NewIBFT(
					mockLogger{},
					&mockBackend{
						isValidBlockFn:         isValidProposal,
						isValidProposalHashFn:  isValidProposalHash,
						isValidSenderFn:        nil,
						isValidCommittedSealFn: nil,
						isProposerFn:           c.isProposer,

						idFn: node.addr,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,

						insertBlockFn: nil,
						hasQuorumFn:   c.hasQuorumFn,
					},

					&mockTransport{multicastFn: c.gossip},
				)
			}
		},
	)

	err := cluster.progressToHeight(5*time.Second, 5)
	assert.NoError(t, err, "unable to reach height: %w", err)

	assert.Equal(t, uint64(5), cluster.latestHeight)

	offline := int(cluster.maxFaulty()) + 1

	cluster.stopN(offline)

	assert.Error(t, cluster.progressToHeight(2*time.Second, 10))
	assert.Equal(t, uint64(5), cluster.latestHeight)

	cluster.startN(offline)

	assert.NoError(t, cluster.progressToHeight(5*time.Second, 10))
	assert.Equal(t, uint64(10), cluster.latestHeight)
}

/*
Scenario:
1. Cluster can reach height 5
2. Stop MaxFaulty nodes
3. Cluster can still reach height 10
*/
func TestDropMaxFaulty(t *testing.T) {
	t.Parallel()

	cluster := newCluster(
		5,
		func(c *cluster) {
			for _, node := range c.nodes {
				node.core = NewIBFT(
					mockLogger{},
					&mockBackend{
						isValidBlockFn:         isValidProposal,
						isValidProposalHashFn:  isValidProposalHash,
						isValidSenderFn:        nil,
						isValidCommittedSealFn: nil,
						isProposerFn:           c.isProposer,

						idFn: node.addr,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,

						insertBlockFn: nil,
						hasQuorumFn:   c.hasQuorumFn,
					},

					&mockTransport{multicastFn: c.gossip},
				)
			}
		},
	)

	err := cluster.progressToHeight(5*time.Second, 5)
	assert.NoError(t, err, "unable to reach height: %w", err)

	assert.Equal(t, uint64(5), cluster.latestHeight)

	cluster.stopN(int(cluster.maxFaulty()))

	// higher timeout due to round-robin proposer selection
	assert.NoError(t, cluster.progressToHeight(20*time.Second, 10))
	assert.Equal(t, uint64(10), cluster.latestHeight)
}
