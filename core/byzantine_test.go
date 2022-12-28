package core

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestByzantineBehaviour(t *testing.T) {
	t.Parallel()

	t.Run("malicious hash in proposal", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},

							idFn: node.addr,

							buildProposalFn: buildValidProposal,
							buildPrePrepareMessageFn: func(proposal []byte,
								rcc *proto.RoundChangeCertificate,
								view *proto.View) *proto.Message {
								proposalHash := validProposalHash
								if currentNode.byzantine {
									proposalHash = []byte("invalid proposal hash")
								}

								return buildBasicPreprepareMessage(
									proposal,
									proposalHash,
									rcc,
									currentNode.address,
									view,
								)
							},
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

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(20*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious hash in prepare", func(t *testing.T) {
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

							buildProposalFn:          buildValidProposal,
							buildPrePrepareMessageFn: node.buildPrePrepare,
							buildPrepareMessageFn: func(_ []byte, view *proto.View) *proto.Message {
								proposalHash := validProposalHash
								if currentNode.byzantine {
									proposalHash = []byte("invalid proposal hash")
								}

								return buildBasicPrepareMessage(
									proposalHash,
									currentNode.address,
									view,
								)
							},
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

		err := cluster.progressToHeight(10*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(10*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious +1 round in proposal", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},

							idFn: node.addr,

							buildProposalFn: buildValidProposal,
							buildPrePrepareMessageFn: func(
								proposal []byte,
								certificate *proto.RoundChangeCertificate,
								view *proto.View,
							) *proto.Message {
								fmt.Println(currentNode.address)
								currentNode.core.log.Debug(fmt.Sprint("TERE ", view, currentNode.byzantine))
								if currentNode.byzantine {
									view.Round = uint64(rand.Int())
								}

								return buildBasicPreprepareMessage(
									proposal,
									validProposalHash,
									certificate,
									currentNode.address,
									view,
								)
							},
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

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		// Max tolerant byzantine
		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(40*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious +1 round in rcc", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},
							idFn: node.addr,

							buildProposalFn:          buildValidProposal,
							buildPrePrepareMessageFn: node.buildPrePrepare,
							buildPrepareMessageFn:    node.buildPrepare,
							buildCommitMessageFn:     node.buildCommit,
							buildRoundChangeMessageFn: func(proposal []byte,
								rcc *proto.PreparedCertificate,
								view *proto.View) *proto.Message {
								if currentNode.byzantine {
									view.Round++
								}

								return buildBasicRoundChangeMessage(
									proposal,
									rcc,
									view,
									currentNode.address,
								)
							},

							insertBlockFn: nil,
							hasQuorumFn:   c.hasQuorumFn,
						},

						&mockTransport{multicastFn: c.gossip},
					)
				}
			},
		)

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(30*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious +1 round in rcc and in proposal", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},
							idFn: node.addr,

							buildProposalFn: buildValidProposal,
							buildPrePrepareMessageFn: func(
								proposal []byte,
								certificate *proto.RoundChangeCertificate,
								view *proto.View,
							) *proto.Message {
								fmt.Println(currentNode.address)
								currentNode.core.log.Debug(fmt.Sprint("TERE ", view, currentNode.byzantine))
								if currentNode.byzantine {
									view.Round = uint64(rand.Int())
								}

								return buildBasicPreprepareMessage(
									proposal,
									validProposalHash,
									certificate,
									currentNode.address,
									view,
								)
							},
							buildPrepareMessageFn: node.buildPrepare,
							buildCommitMessageFn:  node.buildCommit,
							buildRoundChangeMessageFn: func(proposal []byte,
								rcc *proto.PreparedCertificate,
								view *proto.View) *proto.Message {
								if currentNode.byzantine {
									view.Round++
								}

								return buildBasicRoundChangeMessage(
									proposal,
									rcc,
									view,
									currentNode.address,
								)
							},

							insertBlockFn: nil,
							hasQuorumFn:   c.hasQuorumFn,
						},

						&mockTransport{multicastFn: c.gossip},
					)
				}
			},
		)

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(30*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious +1 round in rcc and bad hash in proposal", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},
							idFn: node.addr,

							buildProposalFn: buildValidProposal,
							buildPrePrepareMessageFn: func(proposal []byte,
								rcc *proto.RoundChangeCertificate,
								view *proto.View) *proto.Message {
								proposalHash := validProposalHash
								if currentNode.byzantine {
									proposalHash = []byte("invalid proposal hash")
								}

								return buildBasicPreprepareMessage(
									proposal,
									proposalHash,
									rcc,
									currentNode.address,
									view,
								)
							},
							buildPrepareMessageFn: node.buildPrepare,
							buildCommitMessageFn:  node.buildCommit,
							buildRoundChangeMessageFn: func(proposal []byte,
								rcc *proto.PreparedCertificate,
								view *proto.View) *proto.Message {
								if currentNode.byzantine {
									view.Round++
								}

								return buildBasicRoundChangeMessage(
									proposal,
									rcc,
									view,
									currentNode.address,
								)
							},

							insertBlockFn: nil,
							hasQuorumFn:   c.hasQuorumFn,
						},

						&mockTransport{multicastFn: c.gossip},
					)
				}
			},
		)

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(30*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious +1 round in rcc and bad hash in prepare", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},
							idFn: node.addr,

							buildProposalFn:          buildValidProposal,
							buildPrePrepareMessageFn: node.buildPrePrepare,
							buildPrepareMessageFn: func(_ []byte, view *proto.View) *proto.Message {
								proposalHash := validProposalHash
								if currentNode.byzantine {
									proposalHash = []byte("invalid proposal hash")
								}

								return buildBasicPrepareMessage(
									proposalHash,
									currentNode.address,
									view,
								)
							},
							buildCommitMessageFn: node.buildCommit,
							buildRoundChangeMessageFn: func(proposal []byte,
								rcc *proto.PreparedCertificate,
								view *proto.View) *proto.Message {
								if currentNode.byzantine {
									view.Round++
								}

								return buildBasicRoundChangeMessage(
									proposal,
									rcc,
									view,
									currentNode.address,
								)
							},

							insertBlockFn: nil,
							hasQuorumFn:   c.hasQuorumFn,
						},

						&mockTransport{multicastFn: c.gossip},
					)
				}
			},
		)

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(30*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})

	t.Run("malicious +1 round in rcc and bad commit seal", func(t *testing.T) {
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
							isProposerFn: func(from []byte, height uint64, round uint64) bool {
								if round == 0 {
									return false
								}

								return bytes.Equal(
									from,
									c.addresses()[int(round)%len(c.addresses())],
								)
							},
							idFn: node.addr,

							buildProposalFn:          buildValidProposal,
							buildPrePrepareMessageFn: node.buildPrePrepare,
							buildPrepareMessageFn:    node.buildPrepare,
							buildCommitMessageFn: func(_ []byte, view *proto.View) *proto.Message {
								committedSeal := validCommittedSeal
								if currentNode.byzantine {
									committedSeal = []byte("invalid committed seal")
								}

								return buildBasicCommitMessage(
									validProposalHash,
									committedSeal,
									currentNode.address,
									view,
								)
							},
							buildRoundChangeMessageFn: func(proposal []byte,
								rcc *proto.PreparedCertificate,
								view *proto.View) *proto.Message {
								if currentNode.byzantine {
									view.Round++
								}

								return buildBasicRoundChangeMessage(
									proposal,
									rcc,
									view,
									currentNode.address,
								)
							},

							insertBlockFn: nil,
							hasQuorumFn:   c.hasQuorumFn,
						},

						&mockTransport{multicastFn: c.gossip},
					)
				}
			},
		)

		err := cluster.progressToHeight(20*time.Second, 1)
		assert.NoError(t, err, "unable to reach height: %w", err)
		assert.Equal(t, uint64(1), cluster.latestHeight)

		cluster.makeNByzantine(int(cluster.maxFaulty()))
		assert.NoError(t, cluster.progressToHeight(30*time.Second, 2))
		assert.Equal(t, uint64(2), cluster.latestHeight)
	})
}
