package core

import (
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/messages/proto"
)

// Define delegation methods for hooks
type isValidBlockDelegate func([]byte) bool
type isValidMessageDelegate func(*proto.Message) bool
type isProposerDelegate func([]byte, uint64, uint64) bool
type buildProposalDelegate func(uint64) ([]byte, error)
type verifyProposalHashDelegate func([]byte, []byte) error
type isValidCommittedSealDelegate func([]byte, []byte) bool

type buildPrePrepareMessageDelegate func([]byte) *proto.Message
type buildPrepareMessageDelegate func([]byte) *proto.Message
type buildCommitMessageDelegate func([]byte) *proto.Message
type buildRoundChangeMessageDelegate func(uint64, uint64) *proto.Message
type validatorCountDelegate func(blockNumber uint64) uint64
type insertBlockDelegate func([]byte, [][]byte) error
type idDelegate func() []byte

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

func (m mockBackend) HookIsValidBlock(fn isValidBlockDelegate) {
	m.isValidBlockFn = fn
}

func (m mockBackend) IsValidMessage(msg *proto.Message) bool {
	if m.isValidBlockFn != nil {
		return m.isValidMessageFn(msg)
	}

	return true
}

func (m mockBackend) HookIsValidMessage(fn isValidMessageDelegate) {
	m.isValidMessageFn = fn
}

func (m mockBackend) IsProposer(id []byte, sequence, round uint64) bool {
	if m.isProposerFn != nil {
		return m.isProposerFn(id, sequence, round)
	}

	return false
}

func (m mockBackend) HookIsProposer(fn isProposerDelegate) {
	m.isProposerFn = fn
}

func (m mockBackend) BuildProposal(blockNumber uint64) ([]byte, error) {
	if m.buildProposalFn != nil {
		return m.buildProposalFn(blockNumber)
	}

	return nil, nil
}

func (m mockBackend) HookBuildProposal(fn buildProposalDelegate) {
	m.buildProposalFn = fn
}

func (m mockBackend) VerifyProposalHash(proposal, hash []byte) error {
	if m.verifyProposalHashFn != nil {
		return m.verifyProposalHashFn(proposal, hash)
	}

	return nil
}

func (m mockBackend) HookVerifyProposalHash(fn verifyProposalHashDelegate) {
	m.verifyProposalHashFn = fn
}

func (m mockBackend) IsValidCommittedSeal(proposal, seal []byte) bool {
	if m.isValidCommittedSealFn != nil {
		return m.isValidCommittedSealFn(proposal, seal)
	}

	return true
}

func (m mockBackend) HookIsValidCommittedSeal(fn isValidCommittedSealDelegate) {
	m.isValidCommittedSealFn = fn
}

// Define delegation methods for hooks
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

func (t mockTransport) HookMulticast(fn multicastFnDelegate) {
	t.multicastFn = fn
}

// Define delegation methods for hooks
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

func (l mockLogger) HookInfo(fn opLogDelegate) {
	l.infoFn = fn
}

func (l mockLogger) Debug(msg string, args ...interface{}) {
	if l.debugFn != nil {
		l.debugFn(msg, args)
	}
}

func (l mockLogger) HookDebug(fn opLogDelegate) {
	l.debugFn = fn
}

func (l mockLogger) Error(msg string, args ...interface{}) {
	if l.errorFn != nil {
		l.errorFn(msg, args)
	}
}

func (l mockLogger) HookError(fn opLogDelegate) {
	l.errorFn = fn
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

func (m mockBackend) BuildPrePrepareMessage(proposal []byte) *proto.Message {
	if m.buildPrePrepareMessageFn != nil {
		return m.buildPrePrepareMessageFn(proposal)
	}

	return nil
}

func (m mockBackend) HookBuildPrePrepareMessage(fn buildPrePrepareMessageDelegate) {
	m.buildPrePrepareMessageFn = fn
}

func (m mockBackend) BuildPrepareMessage(proposal []byte) *proto.Message {
	if m.buildPrepareMessageFn != nil {
		return m.buildPrepareMessageFn(proposal)
	}

	return nil
}

func (m mockBackend) HookBuildPrepareMessage(fn buildPrepareMessageDelegate) {
	m.buildPrepareMessageFn = fn
}

func (m mockBackend) BuildCommitMessage(proposal []byte) *proto.Message {
	if m.buildCommitMessageFn != nil {
		return m.buildCommitMessageFn(proposal)
	}

	return nil
}

func (m mockBackend) HookBuildCommitMessage(fn buildCommitMessageDelegate) {
	m.buildCommitMessageFn = fn
}

func (m mockBackend) BuildRoundChangeMessage(height uint64, round uint64) *proto.Message {
	if m.buildRoundChangeMessageFn != nil {
		return m.buildRoundChangeMessageFn(height, round)
	}

	return nil
}

func (m mockBackend) HookBuildRoundChangeMessage(fn buildRoundChangeMessageDelegate) {
	m.buildRoundChangeMessageFn = fn
}
