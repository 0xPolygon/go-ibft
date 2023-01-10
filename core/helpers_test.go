package core

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0xPolygon/go-ibft/messages/proto"
)

/*	HELPERS */

var (
	validProposal      = []byte("valid proposal")
	validProposalHash  = []byte("valid proposal hash")
	validCommittedSeal = []byte("valid committed seal")
)

func isValidProposal(newProposal []byte) bool {
	return bytes.Equal(newProposal, validProposal)
}

func buildValidProposal(_ *proto.View) []byte {
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
	core      *IBFT
	address   []byte
	offline   bool
	byzantine bool
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
		sequenceDone := c.runSequence(ctx, current)

		select {
		case <-sequenceDone:
			c.latestHeight = current
		case <-ctx.Done():
			// wait for the worker threads to return
			<-sequenceDone

			return errors.New("timeout")
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
	addresses := make([][]byte, len(c.nodes))
	for i, node := range c.nodes {
		addresses[i] = node.address
	}

	return addresses
}

func (c *cluster) hasQuorumFn(blockNumber uint64, messages []*proto.Message, msgType proto.MessageType) bool {
	return commonHasQuorumFn(uint64(len(c.nodes)))(blockNumber, messages, msgType)
}

func (c *cluster) isProposer(
	from []byte,
	height,
	round uint64,
) bool {
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

func (c *cluster) makeNByzantine(num int) {
	for i := 0; i < num; i++ {
		c.nodes[i].byzantine = true
	}
}

func (c *cluster) stopN(num int) {
	for i := 0; i < num; i++ {
		c.nodes[i].offline = true
	}
}

func (c *cluster) startN(num int) {
	for i := 0; i < num; i++ {
		c.nodes[i].offline = false
	}
}
