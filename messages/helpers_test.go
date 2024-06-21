package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/go-ibft/messages/proto"
)

var proposalHash = []byte("proposal hash")

func TestMessages_ExtractCommittedSeals(t *testing.T) {
	t.Parallel()

	var (
		committedSeal = []byte("committed seal")
	)

	createCommitMessage := func(signer string) *proto.IbftMessage {
		return &proto.IbftMessage{
			Type: proto.MessageType_COMMIT,
			Payload: &proto.IbftMessage_CommitData{
				CommitData: &proto.CommitMessage{
					CommittedSeal: committedSeal,
				},
			},
			From: []byte(signer),
		}
	}

	createWrongMessage := func(signer string, msgType proto.MessageType) *proto.IbftMessage {
		return &proto.IbftMessage{
			Type: msgType,
		}
	}

	createCommittedSeal := func(from string) *CommittedSeal {
		return &CommittedSeal{
			Signer:    []byte(from),
			Signature: committedSeal,
		}
	}

	tests := []struct {
		name     string
		messages []*proto.IbftMessage
		expected []*CommittedSeal
		err      error
	}{
		{
			name: "contains only valid COMMIT messages",
			messages: []*proto.IbftMessage{
				createCommitMessage("signer1"),
				createCommitMessage("signer2"),
			},
			expected: []*CommittedSeal{
				createCommittedSeal("signer1"),
				createCommittedSeal("signer2"),
			},
			err: nil,
		},
		{
			name: "contains wrong type messages",
			messages: []*proto.IbftMessage{
				createCommitMessage("signer1"),
				createWrongMessage("signer2", proto.MessageType_PREPREPARE),
			},
			expected: nil,
			err:      ErrWrongCommitMessageType,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			seals, err := ExtractCommittedSeals(test.messages)

			assert.Equal(t, test.expected, seals)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestMessages_ExtractCommitHash(t *testing.T) {
	t.Parallel()

	commitHash := []byte("commit hash")

	testTable := []struct {
		name               string
		expectedCommitHash []byte
		message            *proto.IbftMessage
	}{
		{
			"valid message",
			commitHash,
			&proto.IbftMessage{
				Type: proto.MessageType_COMMIT,
				Payload: &proto.IbftMessage_CommitData{
					CommitData: &proto.CommitMessage{
						ProposalHash: commitHash,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedCommitHash,
				ExtractCommitHash(testCase.message),
			)
		})
	}
}

func TestMessages_ExtractProposal(t *testing.T) {
	t.Parallel()

	proposal := &proto.Proposal{}

	testTable := []struct {
		name             string
		expectedProposal *proto.Proposal
		message          *proto.IbftMessage
	}{
		{
			"valid message",
			proposal,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.IbftMessage_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal: proposal,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedProposal,
				ExtractProposal(testCase.message),
			)
		})
	}
}

func TestMessages_ExtractProposalHash(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name                 string
		expectedProposalHash []byte
		message              *proto.IbftMessage
	}{
		{
			"valid message",
			proposalHash,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.IbftMessage_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						ProposalHash: proposalHash,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedProposalHash,
				ExtractProposalHash(testCase.message),
			)
		})
	}
}

func TestMessages_ExtractRCC(t *testing.T) {
	t.Parallel()

	rcc := &proto.RoundChangeCertificate{
		RoundChangeMessages: nil,
	}

	testTable := []struct {
		name        string
		expectedRCC *proto.RoundChangeCertificate
		message     *proto.IbftMessage
	}{
		{
			"valid message",
			rcc,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.IbftMessage_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Certificate: rcc,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedRCC,
				ExtractRoundChangeCertificate(testCase.message),
			)
		})
	}
}

func TestMessages_ExtractPrepareHash(t *testing.T) {
	t.Parallel()

	prepareHash := []byte("prepare hash")

	testTable := []struct {
		name                string
		expectedPrepareHash []byte
		message             *proto.IbftMessage
	}{
		{
			"valid message",
			prepareHash,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPARE,
				Payload: &proto.IbftMessage_PrepareData{
					PrepareData: &proto.PrepareMessage{
						ProposalHash: prepareHash,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedPrepareHash,
				ExtractPrepareHash(testCase.message),
			)
		})
	}
}

func TestMessages_ExtractLatestPC(t *testing.T) {
	t.Parallel()

	latestPC := &proto.PreparedCertificate{
		ProposalMessage: nil,
		PrepareMessages: nil,
	}

	testTable := []struct {
		name       string
		expectedPC *proto.PreparedCertificate
		message    *proto.IbftMessage
	}{
		{
			"valid message",
			latestPC,
			&proto.IbftMessage{
				Type: proto.MessageType_ROUND_CHANGE,
				Payload: &proto.IbftMessage_RoundChangeData{
					RoundChangeData: &proto.RoundChangeMessage{
						LatestPreparedCertificate: latestPC,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedPC,
				ExtractLatestPC(testCase.message),
			)
		})
	}
}

func TestMessages_ExtractLPPB(t *testing.T) {
	t.Parallel()

	lastPreparedProposal := &proto.Proposal{}

	testTable := []struct {
		name         string
		expectedLPPB *proto.Proposal
		message      *proto.IbftMessage
	}{
		{
			"valid message",
			lastPreparedProposal,
			&proto.IbftMessage{
				Type: proto.MessageType_ROUND_CHANGE,
				Payload: &proto.IbftMessage_RoundChangeData{
					RoundChangeData: &proto.RoundChangeMessage{
						LastPreparedProposal: lastPreparedProposal,
					},
				},
			},
		},
		{
			"invalid message",
			nil,
			&proto.IbftMessage{
				Type: proto.MessageType_PREPREPARE,
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedLPPB,
				ExtractLastPreparedProposal(testCase.message),
			)
		})
	}
}

func TestMessages_HasUniqueSenders(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name      string
		messages  []*proto.IbftMessage
		hasUnique bool
	}{
		{
			"empty messages",
			nil,
			false,
		},
		{
			"non unique senders",
			[]*proto.IbftMessage{
				{
					From: []byte("node 1"),
				},
				{
					From: []byte("node 1"),
				},
			},
			false,
		},
		{
			"unique senders",
			[]*proto.IbftMessage{
				{
					From: []byte("node 1"),
				},
				{
					From: []byte("node 2"),
				},
			},
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.hasUnique,
				HasUniqueSenders(testCase.messages),
			)
		})
	}
}

func TestMessages_HaveSameProposalHash(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name     string
		messages []*proto.IbftMessage
		haveSame bool
	}{
		{
			"empty messages",
			nil,
			false,
		},
		{
			"invalid message type",
			[]*proto.IbftMessage{
				{
					Type: proto.MessageType_ROUND_CHANGE,
					View: &proto.View{
						Height: 1,
						Round:  1,
					},
					From: []byte("node 1"),
				},
			},
			false,
		},
		{
			"hash mismatch",
			[]*proto.IbftMessage{
				{
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					View: &proto.View{
						Height: 1,
						Round:  1,
					},
					From: []byte("node 1"),
				},
				{
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: []byte("differing hash"),
						},
					},
					View: &proto.View{
						Height: 1,
						Round:  1,
					},
					From: []byte("node 2"),
				},
			},
			false,
		},
		{
			"hash match",
			[]*proto.IbftMessage{
				{
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					View: &proto.View{
						Height: 1,
						Round:  1,
					},
					From: []byte("node 1"),
				},
				{
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					View: &proto.View{
						Height: 1,
						Round:  1,
					},
					From: []byte("node 2"),
				},
			},
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.haveSame,
				AreValidPCMessages(testCase.messages, 1, 2),
			)
		})
	}
}

func TestMessages_AllHaveLowerRond(t *testing.T) {
	t.Parallel()

	round := uint64(1)

	testTable := []struct {
		name      string
		messages  []*proto.IbftMessage
		round     uint64
		haveLower bool
	}{
		{
			"empty messages",
			nil,
			0,
			false,
		},
		{
			"not same lower round",
			[]*proto.IbftMessage{
				{
					View: &proto.View{
						Height: 0,
						Round:  round,
					},
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 1"),
				},
				{
					View: &proto.View{
						Height: 0,
						Round:  round + 1,
					},
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 2"),
				},
			},
			round,
			false,
		},
		{
			"same higher round",
			[]*proto.IbftMessage{
				{
					View: &proto.View{
						Height: 0,
						Round:  round + 1,
					},
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 1"),
				},
				{
					View: &proto.View{
						Height: 0,
						Round:  round + 1,
					},
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 2"),
				},
			},
			round,
			false,
		},
		{
			"lower round match",
			[]*proto.IbftMessage{
				{
					View: &proto.View{
						Height: 0,
						Round:  round,
					},
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 1"),
				},
				{
					View: &proto.View{
						Height: 0,
						Round:  round,
					},
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 2"),
				},
			},
			2,
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.haveLower,
				AreValidPCMessages(
					testCase.messages,
					0,
					testCase.round,
				),
			)
		})
	}
}

func TestMessages_AllHaveSameHeight(t *testing.T) {
	t.Parallel()

	height := uint64(1)

	testTable := []struct {
		name     string
		messages []*proto.IbftMessage
		haveSame bool
	}{
		{
			"empty messages",
			nil,
			false,
		},
		{
			"not same height",
			[]*proto.IbftMessage{
				{
					View: &proto.View{
						Height: height - 1,
					},
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 1"),
				},
				{
					View: &proto.View{
						Height: height,
					},
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 2"),
				},
			},
			false,
		},
		{
			"same height",
			[]*proto.IbftMessage{
				{
					View: &proto.View{
						Height: height,
						Round:  1,
					},
					Type: proto.MessageType_PREPREPARE,
					Payload: &proto.IbftMessage_PreprepareData{
						PreprepareData: &proto.PrePrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 1"),
				},
				{
					View: &proto.View{
						Height: height,
						Round:  1,
					},
					Type: proto.MessageType_PREPARE,
					Payload: &proto.IbftMessage_PrepareData{
						PrepareData: &proto.PrepareMessage{
							ProposalHash: proposalHash,
						},
					},
					From: []byte("node 2"),
				},
			},
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.haveSame,
				AreValidPCMessages(
					testCase.messages,
					height,
					2,
				),
			)
		})
	}
}
