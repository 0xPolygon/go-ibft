// Package core implements IBFT consensus
// backend.go defines interfaces of backend, that performs detailed procedure rather than consensus
package core

import (
	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
)

// MessageConstructor defines a message constructor interface
type MessageConstructor interface {
	// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in proposal
	BuildPrePrepareMessage(
		rawProposal []byte,
		certificate *proto.RoundChangeCertificate,
		view *proto.View,
	) *proto.Message

	// BuildPrepareMessage builds a PREPARE message based on the passed in proposal
	BuildPrepareMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildCommitMessage builds a COMMIT message based on the passed in proposal
	BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in proposal
	BuildRoundChangeMessage(
		proposal *proto.Proposal,
		certificate *proto.PreparedCertificate,
		view *proto.View,
	) *proto.Message
}

// Verifier defines the verifier interface
type Verifier interface {
	// IsValidBlock checks if the proposed block is child of parent
	IsValidBlock(rawProposal []byte) bool

	// IsValidSender checks if signature is from sender
	IsValidSender(msg *proto.Message) bool

	// IsProposer checks if the passed in ID is the Proposer for current view (sequence, round)
	IsProposer(id []byte, height, round uint64) bool

	// IsValidProposalHash checks if the hash matches the proposal
	IsValidProposalHash(proposal *proto.Proposal, hash []byte) bool

	// IsValidCommittedSeal checks if the seal for the proposal is valid
	IsValidCommittedSeal(proposal []byte, committedSeal *messages.CommittedSeal) bool
}

// Backend defines an interface all backend implementations
// need to implement
type Backend interface {
	MessageConstructor
	Verifier

	// BuildProposal builds a new proposal for the height
	BuildProposal(height uint64) []byte

	// InsertProposal inserts a proposal with the specified committed seals
	// the reason why we are including round here is because a single committedSeal has signed the tuple of (EB, r)
	InsertProposal(proposal *proto.Proposal, committedSeals []*messages.CommittedSeal)

	// ID returns the validator's ID
	ID() []byte

	// HasQuorum returns true if the quorum is reached
	// for the specified block height.
	HasQuorum(blockNumber uint64, msgs []*proto.Message, msgType proto.MessageType) bool
}
