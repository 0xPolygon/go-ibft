package core

import (
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/messages/proto"
)

// Define delegation methods
type isValidBlockDelegate func([]byte) bool
type isValidMessageDelegate func(*proto.Message) bool
type isProposerDelegate func([]byte, uint64, uint64) bool
type buildProposalDelegate func(uint64) ([]byte, error)
type verifyProposalHashDelegate func([]byte, []byte) error
type isValidCommittedSealDelegate func([]byte, []byte) bool

type buildPrePrepareMessageDelegate func([]byte, *proto.View) *proto.Message
type buildPrepareMessageDelegate func([]byte, *proto.View) *proto.Message
type buildCommitMessageDelegate func([]byte, *proto.View) *proto.Message
type buildRoundChangeMessageDelegate func(uint64, uint64) *proto.Message

type validatorCountDelegate func(blockNumber uint64) uint64
type insertBlockDelegate func([]byte, [][]byte) error
type idDelegate func() []byte
type allowedFaultyDelegate func() uint64

// mockBackend is the mock backend structure that is configurable
type mockBackend struct {
	isValidBlockFn         isValidBlockDelegate
	isValidMessageFn       isValidMessageDelegate
	isProposerFn           isProposerDelegate
	buildProposalFn        buildProposalDelegate
	verifyProposalHashFn   verifyProposalHashDelegate
	isValidCommittedSealFn isValidCommittedSealDelegate

	buildPrePrepareMessageFn  buildPrePrepareMessageDelegate
	buildPrepareMessageFn     buildPrepareMessageDelegate
	buildCommitMessageFn      buildCommitMessageDelegate
	buildRoundChangeMessageFn buildRoundChangeMessageDelegate
	validatorCountFn          validatorCountDelegate
	insertBlockFn             insertBlockDelegate
	idFn                      idDelegate
	allowedFaultyFn           allowedFaultyDelegate
}

func (m mockBackend) ID() []byte {
	return m.idFn()
}

func (m mockBackend) InsertBlock(proposal []byte, seals [][]byte) error {
	return m.insertBlockFn(proposal, seals)
}

func (m mockBackend) ValidatorCount(blockNumber uint64) uint64 {
	return m.validatorCountFn(blockNumber)
}

func (m mockBackend) IsValidBlock(block []byte) bool {
	if m.isValidBlockFn != nil {
		return m.isValidBlockFn(block)
	}

	return true
}

func (m mockBackend) IsValidMessage(msg *proto.Message) bool {
	if m.isValidBlockFn != nil {
		return m.isValidMessageFn(msg)
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

	getPrePrepareMessageFn       func(view *proto.View) *messages.PrePrepareMessage
	getPrepareMessagesFn         func(view *proto.View) []*messages.PrepareMessage
	getCommitMessagesFn          func(view *proto.View) []*messages.CommitMessage
	getRoundChangeMessagesFn     func(view *proto.View) []*messages.RoundChangeMessage
	getMostRoundChangeMessagesFn func() []*messages.RoundChangeMessage
	getAndPrunePrepareMessagesFn func(view *proto.View) []*proto.Message
	getAndPruneCommitMessagesFn  func(view *proto.View) []*proto.Message
}

func (m mockMessages) AddMessage(msg *proto.Message) {
	m.addMessageFn(msg)
}

func (m mockMessages) NumMessages(view *proto.View, messageType proto.MessageType) int {
	return m.numMessagesFn(view, messageType)
}

func (m mockMessages) PruneByHeight(view *proto.View) {
	m.pruneByHeightFn(view)
}

func (m mockMessages) PruneByRound(view *proto.View) {
	m.pruneByRoundFn(view)
}

func (m mockMessages) GetPrePrepareMessage(view *proto.View) *messages.PrePrepareMessage {
	return m.getPrePrepareMessageFn(view)
}

// GetPrepareMessages returns all PREPARE messages, if any
func (m mockMessages) GetPrepareMessages(view *proto.View) []*messages.PrepareMessage {
	return m.getPrepareMessagesFn(view)
}

// GetCommitMessages returns all COMMIT messages, if any
func (m mockMessages) GetCommitMessages(view *proto.View) []*messages.CommitMessage {
	return m.getCommitMessagesFn(view)
}

// GetRoundChangeMessages returns all ROUND_CHANGE message, if any
func (m mockMessages) GetRoundChangeMessages(view *proto.View) []*messages.RoundChangeMessage {
	return m.getRoundChangeMessagesFn(view)
}

func (m mockMessages) GetMostRoundChangeMessages() []*messages.RoundChangeMessage {
	return m.getMostRoundChangeMessagesFn()
}

func (m mockMessages) GetAndPrunePrepareMessages(view *proto.View) []*proto.Message {
	return m.getAndPrunePrepareMessagesFn(view)
}

func (m mockMessages) GetAndPruneCommitMessages(view *proto.View) []*proto.Message {
	return m.getAndPruneCommitMessagesFn(view)
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
