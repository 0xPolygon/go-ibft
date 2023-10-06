package messages

import (
	"bytes"
	"errors"

	"github.com/0xPolygon/go-ibft/messages/proto"
)

var (
	// ErrWrongCommitMessageType is an error indicating wrong type in commit messages
	ErrWrongCommitMessageType = errors.New("wrong type message is included in COMMIT messages")
)

// CommittedSeal Validator proof of signing a committed proposal
type CommittedSeal struct {
	Signer    []byte
	Signature []byte
}

// ExtractCommittedSeals extracts the committed seals from the passed in messages
func ExtractCommittedSeals(commitMessages []*proto.Message) ([]*CommittedSeal, error) {
	committedSeals := make([]*CommittedSeal, 0)

	for _, commitMessage := range commitMessages {
		if commitMessage.Type != proto.MessageType_COMMIT {
			// safe check
			return nil, ErrWrongCommitMessageType
		}

		committedSeals = append(committedSeals, ExtractCommittedSeal(commitMessage))
	}

	return committedSeals, nil
}

// ExtractCommittedSeal extracts the committed seal from the passed in message
func ExtractCommittedSeal(commitMessage *proto.Message) *CommittedSeal {
	commitData, _ := commitMessage.Payload.(*proto.Message_CommitData)

	return &CommittedSeal{
		Signer:    commitMessage.From,
		Signature: commitData.CommitData.CommittedSeal,
	}
}

// ExtractCommitHash extracts the commit proposal hash from the passed in message
func ExtractCommitHash(commitMessage *proto.Message) []byte {
	if commitMessage.Type != proto.MessageType_COMMIT {
		return nil
	}

	commitData, _ := commitMessage.Payload.(*proto.Message_CommitData)

	return commitData.CommitData.ProposalHash
}

// ExtractProposal extracts the (rawData,r) proposal from the passed in message
func ExtractProposal(proposalMessage *proto.Message) *proto.Proposal {
	if proposalMessage.Type != proto.MessageType_PREPREPARE {
		return nil
	}

	preprepareData, _ := proposalMessage.Payload.(*proto.Message_PreprepareData)

	return preprepareData.PreprepareData.Proposal
}

// ExtractProposalHash extracts the proposal hash from the passed in message
func ExtractProposalHash(proposalMessage *proto.Message) []byte {
	if proposalMessage.Type != proto.MessageType_PREPREPARE {
		return nil
	}

	preprepareData, _ := proposalMessage.Payload.(*proto.Message_PreprepareData)

	return preprepareData.PreprepareData.ProposalHash
}

// ExtractRoundChangeCertificate extracts the RCC from the passed in message
func ExtractRoundChangeCertificate(proposalMessage *proto.Message) *proto.RoundChangeCertificate {
	if proposalMessage.Type != proto.MessageType_PREPREPARE {
		return nil
	}

	preprepareData, _ := proposalMessage.Payload.(*proto.Message_PreprepareData)

	return preprepareData.PreprepareData.Certificate
}

// ExtractPrepareHash extracts the prepare proposal hash from the passed in message
func ExtractPrepareHash(prepareMessage *proto.Message) []byte {
	if prepareMessage.Type != proto.MessageType_PREPARE {
		return nil
	}

	prepareData, _ := prepareMessage.Payload.(*proto.Message_PrepareData)

	return prepareData.PrepareData.ProposalHash
}

// ExtractLatestPC extracts the latest PC from the passed in message
func ExtractLatestPC(roundChangeMessage *proto.Message) *proto.PreparedCertificate {
	if roundChangeMessage.Type != proto.MessageType_ROUND_CHANGE {
		return nil
	}

	rcData, _ := roundChangeMessage.Payload.(*proto.Message_RoundChangeData)

	return rcData.RoundChangeData.LatestPreparedCertificate
}

// ExtractLastPreparedProposal extracts the latest prepared proposal from the passed in message
func ExtractLastPreparedProposal(roundChangeMessage *proto.Message) *proto.Proposal {
	if roundChangeMessage.Type != proto.MessageType_ROUND_CHANGE {
		return nil
	}

	rcData, _ := roundChangeMessage.Payload.(*proto.Message_RoundChangeData)

	return rcData.RoundChangeData.LastPreparedProposal
}

// HasUniqueSenders checks if the messages have unique senders
func HasUniqueSenders(messages []*proto.Message) bool {
	if len(messages) < 1 {
		return false
	}

	senderMap := make(map[string]struct{})

	for _, message := range messages {
		key := string(message.From)
		if _, exists := senderMap[key]; exists {
			return false
		}

		senderMap[key] = struct{}{}
	}

	return true
}

// AreValidPCMessages validates PreparedCertificate messages
func AreValidPCMessages(messages []*proto.Message, height uint64, roundLimit uint64) bool {
	if len(messages) < 1 {
		return false
	}

	round := messages[0].View.Round
	senderMap := make(map[string]struct{})

	var hash []byte

	for _, message := range messages {
		// all messages must have the same height
		if message.View.Height != height {
			return false
		}

		// all messages must have the same round that is not greater than rount limit
		if message.View.Round != round || message.View.Round >= roundLimit {
			return false
		}

		// all messages must have the same proposal hash
		extractedHash, ok := extractPCMessageHash(message)
		if hash == nil {
			// No previous hash for comparison,
			// set the first one as the reference, as
			// all of them need to be the same anyway
			hash = extractedHash
		}

		if !ok || !bytes.Equal(hash, extractedHash) {
			return false
		}

		// all messages must have unique senders
		key := string(message.From)
		if _, exists := senderMap[key]; exists {
			return false
		}

		senderMap[key] = struct{}{}
	}

	return true
}

// extractPCMessageHash extracts the hash from a PC message
func extractPCMessageHash(message *proto.Message) ([]byte, bool) {
	switch message.Type {
	case proto.MessageType_PREPREPARE:
		return ExtractProposalHash(message), true
	case proto.MessageType_PREPARE:
		return ExtractPrepareHash(message), true
	case proto.MessageType_COMMIT, proto.MessageType_ROUND_CHANGE:
		return nil, false
	default:
		return nil, false
	}
}
