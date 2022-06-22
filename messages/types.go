package messages

import "github.com/Trapesys/go-ibft/messages/proto"

// PrePrepareMessage is an internal representation
// of the PREPREPARE message
type PrePrepareMessage struct {
	Proposal []byte
}

// PrepareMessage is an internal representation
// of the PREPARE message
type PrepareMessage struct {
	ProposalHash []byte
}

// CommitMessage is an internal representation
// of the COMMIT message
type CommitMessage struct {
	ProposalHash  []byte
	CommittedSeal []byte
}

// RoundChangeMessage is an internal representation
// of the ROUND_CHANGE message
type RoundChangeMessage struct {
	Height uint64
	Round  uint64
}

// toPrePrepareFromProto transforms a proto message to an internal PREPREPARE message
func toPrePrepareFromProto(message *proto.Message) *PrePrepareMessage {
	messagePayload, _ := message.Payload.(*proto.Message_PreprepareData)

	return &PrePrepareMessage{
		Proposal: messagePayload.PreprepareData.Proposal,
	}
}

// toPrepareFromProto transforms a proto message to an internal PREPARE message
func toPrepareFromProto(message *proto.Message) *PrepareMessage {
	messagePayload, _ := message.Payload.(*proto.Message_PrepareData)

	return &PrepareMessage{
		ProposalHash: messagePayload.PrepareData.ProposalHash,
	}
}

// toCommitFromProto transforms a proto message to an internal COMMIT message
func toCommitFromProto(message *proto.Message) *CommitMessage {
	messagePayload, _ := message.Payload.(*proto.Message_CommitData)

	return &CommitMessage{
		ProposalHash:  messagePayload.CommitData.ProposalHash,
		CommittedSeal: messagePayload.CommitData.CommittedSeal,
	}
}

// toRoundChangeFromProto transforms a proto message to an internal COMMIT message
func toRoundChangeFromProto(message *proto.Message) *RoundChangeMessage {
	return &RoundChangeMessage{
		Height: message.View.Height,
		Round:  message.View.Round,
	}
}
