package messages

import "github.com/Trapesys/go-ibft/messages/proto"

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
