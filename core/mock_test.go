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

func (m mockBackend) ValidatorCount(blockNumber uint64) uint64 {
	if m.validatorCountFn != nil {
		return m.validatorCountFn(blockNumber)
	}

	return 0
}

func (m mockBackend) IsValidBlock(block []byte) bool {
	if m.isValidBlockFn != nil {
		return m.isValidBlockFn(block)
	}

	return true
}

func (m mockBackend) IsValidMessage(msg *proto.Message) bool {
	if m.isValidMessageFn != nil {
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
	getMostRoundChangeMessagesFn func(uint64, uint64) []*messages.RoundChangeMessage
	getAndPrunePrepareMessagesFn func(view *proto.View) []*proto.Message
	getAndPruneCommitMessagesFn  func(view *proto.View) []*proto.Message
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

func (m mockMessages) GetPrePrepareMessage(view *proto.View) *messages.PrePrepareMessage {
	if m.getPrePrepareMessageFn != nil {
		return m.getPrePrepareMessageFn(view)
	}

	return nil
}

// GetPrepareMessages returns all PREPARE messages, if any
func (m mockMessages) GetPrepareMessages(view *proto.View) []*messages.PrepareMessage {
	if m.getPrepareMessagesFn != nil {
		return m.getPrepareMessagesFn(view)
	}

	return nil
}

// GetCommitMessages returns all COMMIT messages, if any
func (m mockMessages) GetCommitMessages(view *proto.View) []*messages.CommitMessage {
	if m.getCommitMessagesFn != nil {
		return m.getCommitMessagesFn(view)
	}

	return nil
}

// GetRoundChangeMessages returns all ROUND_CHANGE message, if any
func (m mockMessages) GetRoundChangeMessages(view *proto.View) []*messages.RoundChangeMessage {
	if m.getRoundChangeMessagesFn != nil {
		return m.getRoundChangeMessagesFn(view)
	}

	return nil
}

func (m mockMessages) GetMostRoundChangeMessages(round, height uint64) []*messages.RoundChangeMessage {
	if m.getMostRoundChangeMessagesFn != nil {
		return m.getMostRoundChangeMessagesFn(round, height)
	}

	return nil
}

func (m mockMessages) GetAndPrunePrepareMessages(view *proto.View) []*proto.Message {
	if m.getAndPrunePrepareMessagesFn != nil {
		return m.getAndPrunePrepareMessagesFn(view)
	}

	return nil
}

func (m mockMessages) GetAndPruneCommitMessages(view *proto.View) []*proto.Message {
	if m.getAndPruneCommitMessagesFn != nil {
		return m.getAndPruneCommitMessagesFn(view)
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
