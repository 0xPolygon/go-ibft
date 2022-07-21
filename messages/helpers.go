package messages

import (
	"bytes"
	"github.com/Trapesys/go-ibft/messages/proto"
)

// ExtractCommittedSeals extracts the committed seals from the passed in messages
func ExtractCommittedSeals(commitMessages []*proto.Message) [][]byte {
	committedSeals := make([][]byte, len(commitMessages))

	for index, commitMessage := range commitMessages {
		committedSeals[index] = ExtractCommittedSeal(commitMessage)
	}

	return committedSeals
}

// ExtractCommittedSeal extracts the committed seal from the passed in message
func ExtractCommittedSeal(commitMessage *proto.Message) []byte {
	commitData, _ := commitMessage.Payload.(*proto.Message_CommitData)

	return commitData.CommitData.CommittedSeal
}

// ExtractCommitHash extracts the commit proposal hash from the passed in message
func ExtractCommitHash(commitMessage *proto.Message) []byte {
	commitData, _ := commitMessage.Payload.(*proto.Message_CommitData)

	return commitData.CommitData.ProposalHash
}

// ExtractProposal extracts the proposal from the passed in message
func ExtractProposal(proposalMessage *proto.Message) []byte {
	preprepareData, _ := proposalMessage.Payload.(*proto.Message_PreprepareData)

	return preprepareData.PreprepareData.Proposal
}

// ExtractPrepareHash extracts the prepare proposal hash from the passed in message
func ExtractPrepareHash(prepareMessage *proto.Message) []byte {
	prepareData, _ := prepareMessage.Payload.(*proto.Message_PrepareData)

	return prepareData.PrepareData.ProposalHash
}

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

func HaveSameProposalHash(messages []*proto.Message) bool {
	if len(messages) < 1 {
		return false
	}

	var hash []byte = nil

	for _, message := range messages {
		var extractedHash []byte = nil

		switch message.Type {
		case proto.MessageType_PREPREPARE:
			payload := message.Payload.(*proto.Message_PreprepareData).PreprepareData

			extractedHash = payload.ProposalHash
		case proto.MessageType_PREPARE:
			payload := message.Payload.(*proto.Message_PrepareData).PrepareData

			extractedHash = payload.ProposalHash
		default:
			return false
		}

		if hash == nil {
			hash = extractedHash
		}

		if !bytes.Equal(hash, extractedHash) {
			return false
		}
	}

	return true
}

func AllHaveLowerRound(messages []*proto.Message, round uint64) bool {
	for _, message := range messages {
		if message.View.Round >= round {
			return false
		}
	}

	return true
}
