package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
