package core

import (
	"github.com/Trapesys/go-ibft/messages/proto"
)

// Define delegation methods
type isValidBlockDelegate func([]byte) bool
type isValidSenderDelegate func(*proto.Message) bool
type isProposerDelegate func([]byte, uint64, uint64) bool
type buildProposalDelegate func(uint64) ([]byte, error)
type verifyProposalHashDelegate func([]byte, []byte) error
type isValidCommittedSealDelegate func([]byte, []byte) bool

type buildPrePrepareMessageDelegate func([]byte, *proto.View) *proto.Message
type buildPrepareMessageDelegate func([]byte, *proto.View) *proto.Message
type buildCommitMessageDelegate func([]byte, *proto.View) *proto.Message
type buildRoundChangeMessageDelegate func(uint64, uint64) *proto.Message

type quorumDelegate func(blockHeight uint64) uint64
type insertBlockDelegate func([]byte, [][]byte) error
type idDelegate func() []byte
type allowedFaultyDelegate func() uint64

// mockBackend is the mock backend structure that is configurable
type mockBackend struct {
	isValidBlockFn         isValidBlockDelegate
	isValidSenderFn        isValidSenderDelegate
	isProposerFn           isProposerDelegate
	buildProposalFn        buildProposalDelegate
	verifyProposalHashFn   verifyProposalHashDelegate
	isValidCommittedSealFn isValidCommittedSealDelegate

	buildPrePrepareMessageFn  buildPrePrepareMessageDelegate
	buildPrepareMessageFn     buildPrepareMessageDelegate
	buildCommitMessageFn      buildCommitMessageDelegate
	buildRoundChangeMessageFn buildRoundChangeMessageDelegate
	quorumFn                  quorumDelegate
	insertBlockFn             insertBlockDelegate
	idFn                      idDelegate
	allowedFaultyFn           allowedFaultyDelegate
}

func (m mockBackend) ID() []byte {
	if m.idFn != nil {
		return m.idFn()
	}

	return nil
}

func (m mockBackend) InsertBlock(proposal []byte, seals [][]byte) error {
	if m.insertBlockFn != nil {
		return m.insertBlockFn(proposal, seals)
	}

	return nil
}

func (m mockBackend) Quorum(blockNumber uint64) uint64 {
	if m.quorumFn != nil {
		return m.quorumFn(blockNumber)
	}

	return 0
}

func (m mockBackend) IsValidBlock(block []byte) bool {
	if m.isValidBlockFn != nil {
		return m.isValidBlockFn(block)
	}

	return true
}

func (m mockBackend) IsValidSender(msg *proto.Message) bool {
	if m.isValidSenderFn != nil {
		return m.isValidSenderFn(msg)
	}

	return true
}

func (m mockBackend) IsProposer(id []byte, sequence, round uint64) bool {
	if m.isProposerFn != nil {
		return m.isProposerFn(id, sequence, round)
	}

	return false
}

func (m mockBackend) BuildProposal(blockNumber uint64) ([]byte, error) {
	if m.buildProposalFn != nil {
		return m.buildProposalFn(blockNumber)
	}

	return nil, nil
}

func (m mockBackend) VerifyProposalHash(proposal, hash []byte) error {
	if m.verifyProposalHashFn != nil {
		return m.verifyProposalHashFn(proposal, hash)
	}

	return nil
}

func (m mockBackend) IsValidCommittedSeal(proposal, seal []byte) bool {
	if m.isValidCommittedSealFn != nil {
		return m.isValidCommittedSealFn(proposal, seal)
	}

	return true
}

func (m mockBackend) AllowedFaulty() uint64 {
	if m.allowedFaultyFn != nil {
		return m.allowedFaultyFn()
	}

	return 0
}

// Define delegation methods
type multicastFnDelegate func(*proto.Message)

// mockTransport is the mock transport structure that is configurable
type mockTransport struct {
	multicastFn multicastFnDelegate
}

func (t mockTransport) Multicast(msg *proto.Message) {
	if t.multicastFn != nil {
		t.multicastFn(msg)
	}
}

// Define delegation methods
type opLogDelegate func(string, ...interface{})

// mockLogger is the mock logging structure that is configurable
type mockLogger struct {
	infoFn,
	debugFn,
	errorFn opLogDelegate
}

func (l mockLogger) Info(msg string, args ...interface{}) {
	if l.infoFn != nil {
		l.infoFn(msg, args)
	}
}

func (l mockLogger) Debug(msg string, args ...interface{}) {
	if l.debugFn != nil {
		l.debugFn(msg, args)
	}
}

func (l mockLogger) Error(msg string, args ...interface{}) {
	if l.errorFn != nil {
		l.errorFn(msg, args)
	}
}

type mockMessages struct {
	addMessageFn    func(message *proto.Message)
	numMessagesFn   func(view *proto.View, messageType proto.MessageType) int
	pruneByHeightFn func(view *proto.View)
	pruneByRoundFn  func(view *proto.View)

	getMessages                  func(view *proto.View, messageType proto.MessageType) []*proto.Message
	getMostRoundChangeMessagesFn func(uint64, uint64) []*proto.Message
	getProposal                  func(view *proto.View) []byte
	getCommittedSeals            func(view *proto.View) [][]byte
}

func (m mockMessages) AddMessage(msg *proto.Message) {
	if m.addMessageFn != nil {
		m.addMessageFn(msg)
	}
}

func (m mockMessages) NumMessages(view *proto.View, messageType proto.MessageType) int {
	if m.numMessagesFn != nil {
		return m.numMessagesFn(view, messageType)
	}

	return 0
}

func (m mockMessages) PruneByHeight(view *proto.View) {
	if m.pruneByHeightFn != nil {
		m.pruneByHeightFn(view)
	}
}

func (m mockMessages) PruneByRound(view *proto.View) {
	if m.pruneByRoundFn != nil {
		m.pruneByRoundFn(view)
	}
}

func (m mockMessages) GetMostRoundChangeMessages(round, height uint64) []*proto.Message {
	if m.getMostRoundChangeMessagesFn != nil {
		return m.getMostRoundChangeMessagesFn(round, height)
	}

	return nil
}

func (m mockMessages) GetMessages(view *proto.View, messageType proto.MessageType) []*proto.Message {
	if m.getMessages != nil {
		return m.getMessages(view, messageType)
	}

	return nil
}

func (m mockMessages) GetProposal(view *proto.View) []byte {
	if m.getProposal != nil {
		return m.getProposal(view)
	}

	return nil
}

func (m mockMessages) GetCommittedSeals(view *proto.View) [][]byte {
	if m.getCommittedSeals != nil {
		return m.getCommittedSeals(view)
	}

	return nil
}

func (m mockBackend) BuildPrePrepareMessage(proposal []byte, view *proto.View) *proto.Message {
	if m.buildPrePrepareMessageFn != nil {
		return m.buildPrePrepareMessageFn(proposal, view)
	}

	return nil
}

func (m mockBackend) BuildPrepareMessage(proposal []byte, view *proto.View) *proto.Message {
	if m.buildPrepareMessageFn != nil {
		return m.buildPrepareMessageFn(proposal, view)
	}

	return nil
}

func (m mockBackend) BuildCommitMessage(proposal []byte, view *proto.View) *proto.Message {
	if m.buildCommitMessageFn != nil {
		return m.buildCommitMessageFn(proposal, view)
	}

	return nil
}

func (m mockBackend) BuildRoundChangeMessage(height uint64, round uint64) *proto.Message {
	if m.buildRoundChangeMessageFn != nil {
		return m.buildRoundChangeMessageFn(height, round)
	}

	return nil
}
