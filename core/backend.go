package core

import (
	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
)

// MessageConstructor defines a message constructor interface
// All constructed messages must be signed by a validator for the whole message
type MessageConstructor interface {
	// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in view and proposal
	BuildPrePrepareMessage(
		proposal []byte,
		certificate *proto.RoundChangeCertificate,
		view *proto.View,
	) *proto.Message

	// BuildPrepareMessage builds a PREPARE message based on the passed in view and proposal hash
	BuildPrepareMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildCommitMessage builds a COMMIT message based on the passed in view and proposal hash
	// Must create a committed seal for proposal hash and include into the message
	BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in view,
	// latest prepared proposed block, and latest prepared certificate
	BuildRoundChangeMessage(
		proposal []byte,
		certificate *proto.PreparedCertificate,
		view *proto.View,
	) *proto.Message
}

// Verifier defines the verifier interface
type Verifier interface {
	// IsValidBlock checks if the proposed block is valid and child of the latest block in local chain
	IsValidBlock(block []byte) bool

	// IsValidValidator checks if a signature in message js signed by sender
	// Must check the following things:
	// (1) recover the signature and the signer matches from address in message
	// (2) the signer address is one of the validators at the height in message
	IsValidValidator(msg *proto.Message) bool

	// IsProposer checks if the passed in ID is the Proposer for current view (sequence, round)
	IsProposer(id []byte, height, round uint64) bool

	// IsValidProposalHash checks if the hash matches the proposal
	IsValidProposalHash(proposal, hash []byte) bool

	// IsValidCommittedSeal checks
	// if signature for proposal hash in committed seal is signed by a validator
	IsValidCommittedSeal(proposalHash []byte, committedSeal *messages.CommittedSeal) bool
}

// Backend defines an interface all backend implementations
// need to implement
type Backend interface {
	MessageConstructor
	Verifier

	// BuildProposal builds a new block proposal
	BuildProposal(view *proto.View) []byte

	// InsertBlock inserts a proposal with the specified committed seals
	InsertBlock(proposal []byte, committedSeals []*messages.CommittedSeal)

	// ID returns the validator's ID
	ID() []byte

	// HasQuorum returns true if the quorum is reached
	// for the specified block height.
	HasQuorum(blockNumber uint64, msgs []*proto.Message, msgType proto.MessageType) bool
}
