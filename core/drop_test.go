package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/go-ibft/messages/proto"
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

						idFn:                 node.addr,
						quorumFn:             c.quorum,
						maximumFaultyNodesFn: c.maxFaulty,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,
						insertBlockFn:             nil,
					},

					&mockTransport{multicastFn: c.gossip},
				)
			}
		},
	)

	if err := cluster.progressToHeight(5*time.Second, 5); err != nil {
		t.Fatal("cannot progress to height: err=", err)
	}

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

						idFn:                 node.addr,
						quorumFn:             c.quorum,
						maximumFaultyNodesFn: c.maxFaulty,

						buildProposalFn:           buildValidProposal,
						buildPrePrepareMessageFn:  node.buildPrePrepare,
						buildPrepareMessageFn:     node.buildPrepare,
						buildCommitMessageFn:      node.buildCommit,
						buildRoundChangeMessageFn: node.buildRoundChange,
						insertBlockFn:             nil,
					},

					&mockTransport{multicastFn: c.gossip},
				)
			}
		},
	)

	if err := cluster.progressToHeight(5*time.Second, 5); err != nil {
		t.Fatal("cannot progress to height: err=", err)
	}

	assert.Equal(t, uint64(5), cluster.latestHeight)

	cluster.stopN(int(cluster.maxFaulty()))

	// higher timeout due to round-robin proposer selection
	assert.NoError(t, cluster.progressToHeight(20*time.Second, 10))
	assert.Equal(t, uint64(10), cluster.latestHeight)
}

/*	HELPERS */

var (
	validProposal      = []byte("valid proposal")
	validProposalHash  = []byte("valid proposal hash")
	validCommittedSeal = []byte("valid committed seal")
)

func isValidProposal(newProposal []byte) bool {
	return bytes.Equal(newProposal, validProposal)
}

func buildValidProposal(_ uint64) []byte {
	return validProposal
}

func isValidProposalHash(proposal, proposalHash []byte) bool {
	return bytes.Equal(
		proposal,
		validProposal,
	) && bytes.Equal(
		proposalHash,
		validProposalHash,
	)
}

type node struct {
	core    *IBFT
	address []byte
	offline bool
}

func (n *node) addr() []byte {
	return n.address
}

func (n *node) buildPrePrepare(
	proposal []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	return buildBasicPreprepareMessage(
		proposal,
		validProposalHash,
		certificate,
		n.address,
		view,
	)
}

func (n *node) buildPrepare(
	_ []byte,
	view *proto.View,
) *proto.Message {
	return buildBasicPrepareMessage(
		validProposalHash,
		n.address,
		view,
	)
}

func (n *node) buildCommit(
	_ []byte,
	view *proto.View,
) *proto.Message {
	return buildBasicCommitMessage(
		validProposalHash,
		validCommittedSeal,
		n.address,
		view,
	)
}

func (n *node) buildRoundChange(
	proposal []byte,
	certificate *proto.PreparedCertificate,
	view *proto.View,
) *proto.Message {
	return buildBasicRoundChangeMessage(
		proposal,
		certificate,
		view,
		n.address,
	)
}

func (n *node) runSequence(ctx context.Context, height uint64) {
	if n.offline {
		return
	}

	ctxSequence, cancel := context.WithCancel(ctx)
	defer cancel()

	n.core.RunSequence(ctxSequence, height)
}

type cluster struct {
	nodes []*node
	wg    sync.WaitGroup

	latestHeight uint64
}

func newCluster(num uint64, init func(*cluster)) *cluster {
	c := &cluster{
		nodes:        make([]*node, num),
		wg:           sync.WaitGroup{},
		latestHeight: 0,
	}

	for i, addr := range generateNodeAddresses(num) {
		c.nodes[i] = &node{address: addr}
	}

	init(c)

	return c
}

func (c *cluster) runSequence(ctx context.Context, height uint64) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		for _, n := range c.nodes {
			c.wg.Add(1)

			go func(n *node) {
				defer c.wg.Done()

				n.runSequence(ctx, height)
			}(n)
		}

		c.wg.Wait()
	}()

	return done
}

func (c *cluster) runSequences(ctx context.Context, height uint64) error {
	for current := c.latestHeight + 1; current <= height; current++ {
		select {
		case <-ctx.Done():
			// wait for subcontext to cancel itself
			time.Sleep(time.Second)

			return errors.New("timeout")
		case <-c.runSequence(ctx, current):
			c.latestHeight = current
		}
	}

	return nil
}

func (c *cluster) progressToHeight(timeout time.Duration, height uint64) error {
	if c.latestHeight >= height {
		panic("height already reached")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return c.runSequences(ctx, height)
}

func (c *cluster) addresses() [][]byte {
	addresses := make([][]byte, 0, len(c.nodes))
	for _, node := range c.nodes {
		addresses = append(addresses, node.address)
	}

	return addresses
}

func (c *cluster) quorum(_ uint64) uint64 {
	return quorum(uint64(len(c.nodes)))
}

func (c *cluster) isProposer(
	from []byte,
	height,
	round uint64,
) bool {
	if len(c.nodes) == 0 {
		panic("no nodes")
	}

	addrs := c.addresses()

	return bytes.Equal(
		from,
		addrs[int(height+round)%len(addrs)],
	)
}

func (c *cluster) gossip(msg *proto.Message) {
	for _, node := range c.nodes {
		node.core.AddMessage(msg)
	}
}

func (c *cluster) maxFaulty() uint64 {
	return (uint64(len(c.nodes)) - 1) / 3
}

func (c *cluster) stopN(num int) {
	if num > len(c.nodes) {
		panic(fmt.Sprintf("stop %d, but size is %d", num, len(c.nodes)))
	}

	for i := 0; i < num; i++ {
		c.nodes[i].offline = true
	}
}

func (c *cluster) startN(num int) {
	for i := 0; i < num; i++ {
		c.nodes[i].offline = false
	}
}