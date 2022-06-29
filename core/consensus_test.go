package core

import "testing"

// TestConsensus_ValidFlow tests the following scenario:
// N = 4
//
// - Node 0 is the proposer for block 1, round 0
// - Node 0 proposes a valid block B
// - All nodes go through the consensus states to insert the valid block B
func TestConsensus_ValidFlow(t *testing.T) {
	// TODO implement
}

// TestConsensus_InvalidBlock tests the following scenario:
// N = 4
//
// - Node 0 is the proposer for block 1, round 0
// - Node 0 proposes an invalid block B
// - Other nodes should verify that the block is invalid
// - All nodes should move to round 1, and start a new consensus round
func TestConsensus_InvalidBlock(t *testing.T) {
	// TODO implement
}

// TestConsensus_InvalidSeals tests the following scenario:
// N = 4
//
// - Node 0 is the proposer for block 1, round 0
// - Node 0 proposes an invalid block B
// - Nodes should receive Quorum committed seals, but some of them being invalid,
// such that len(valid committed seals) < Quorum
// - All nodes should move to round 1, and start a new consensus round
func TestConsensus_InvalidSeals(t *testing.T) {
	// TODO implement
}

// TestConsensus_Persistence verifies the persistence problem
// outlined in the following analysis paper:
// https://arxiv.org/pdf/1901.07160.pdf
func TestConsensus_Persistence(t *testing.T) {
	// TODO implement
}

// TestConsensus_Liveness verifies the liveness problem
// outlined in the following analysis paper:
// https://arxiv.org/pdf/1901.07160.pdf
func TestConsensus_Liveness(t *testing.T) {
	// TODO implement
}
