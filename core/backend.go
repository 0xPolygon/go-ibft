package core

import "github.com/Trapesys/go-ibft/messages/proto"

// Backend defines an interface all core implementations
// need to implement
type Backend interface {
	// IsValidBlock checks if the proposed block is child of parent
	IsValidBlock(block []byte) bool

	// IsValidMessage checks if signature is from sender
	IsValidMessage(msg *proto.Message) bool

	// IsProposer checks if the passed in ID is the Proposer for current view (sequence, round)
	IsProposer(id []byte, sequence, round uint64) bool

	// BuildProposal builds a new block proposal
	BuildProposal(blockNumber uint64) ([]byte, error)

	// VerifyProposalHash checks if the hash matches the proposal
	VerifyProposalHash(proposal, hash []byte) error

	// IsValidCommittedSeal checks if the seal for the proposal is valid
	IsValidCommittedSeal(proposal, seal []byte) bool
}
