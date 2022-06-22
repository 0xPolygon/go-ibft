package core

import "github.com/Trapesys/go-ibft/messages/proto"

// Define delegation methods for hooks
type isValidBlockDelegate func([]byte) bool
type isValidMessageDelegate func(*proto.Message) bool
type isProposerDelegate func([]byte, uint64, uint64) bool
type buildProposalDelegate func(uint64) ([]byte, error)
type verifyProposalHashDelegate func([]byte, []byte) error
type isValidCommittedSealDelegate func([]byte, []byte) bool

// mockBackend is the mock core structure that is configurable
type mockBackend struct {
	isValidBlockFn         isValidBlockDelegate
	isValidMessageFn       isValidMessageDelegate
	isProposerFn           isProposerDelegate
	buildProposalFn        buildProposalDelegate
	verifyProposalHashFn   verifyProposalHashDelegate
	isValidCommittedSealFn isValidCommittedSealDelegate
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
