package backend

import "github.com/Trapesys/go-ibft/proto"

// Define delegation methods for hooks
type isValidBlockDelegate func([]byte) bool
type isValidMessageDelegate func(*proto.Message) bool
type isProposerDelegate func([]byte, uint64, uint64) bool
type buildProposalDelegate func(uint64) ([]byte, error)
type verifyProposalHashDelegate func([]byte, []byte) error
type isValidCommittedSealDelegate func([]byte, []byte) bool

// MockBackend is the mock backend structure that is configurable
type MockBackend struct {
	isValidBlockFn         isValidBlockDelegate
	isValidMessageFn       isValidMessageDelegate
	isProposerFn           isProposerDelegate
	buildProposalFn        buildProposalDelegate
	verifyProposalHashFn   verifyProposalHashDelegate
	isValidCommittedSealFn isValidCommittedSealDelegate
}

func (m *MockBackend) IsValidBlock(block []byte) bool {
	if m.isValidBlockFn != nil {
		return m.isValidBlockFn(block)
	}

	return true
}

func (m *MockBackend) HookIsValidBlock(fn isValidBlockDelegate) {
	m.isValidBlockFn = fn
}

func (m *MockBackend) IsValidMessage(msg *proto.Message) bool {
	if m.isValidBlockFn != nil {
		return m.isValidMessageFn(msg)
	}

	return true
}

func (m *MockBackend) HookIsValidMessage(fn isValidMessageDelegate) {
	m.isValidMessageFn = fn
}

func (m *MockBackend) IsProposer(id []byte, sequence, round uint64) bool {
	if m.isProposerFn != nil {
		return m.isProposerFn(id, sequence, round)
	}

	return false
}

func (m *MockBackend) HookIsProposer(fn isProposerDelegate) {
	m.isProposerFn = fn
}

func (m *MockBackend) BuildProposal(blockNumber uint64) ([]byte, error) {
	if m.buildProposalFn != nil {
		return m.buildProposalFn(blockNumber)
	}

	return nil, nil
}

func (m *MockBackend) HookBuildProposal(fn buildProposalDelegate) {
	m.buildProposalFn = fn
}

func (m *MockBackend) VerifyProposalHash(proposal, hash []byte) error {
	if m.verifyProposalHashFn != nil {
		return m.verifyProposalHashFn(proposal, hash)
	}

	return nil
}

func (m *MockBackend) HookVerifyProposalHash(fn verifyProposalHashDelegate) {
	m.verifyProposalHashFn = fn
}

func (m *MockBackend) IsValidCommittedSeal(proposal, seal []byte) bool {
	if m.isValidCommittedSealFn != nil {
		return m.isValidCommittedSealFn(proposal, seal)
	}

	return true
}

func (m *MockBackend) HookIsValidCommittedSeal(fn isValidCommittedSealDelegate) {
	m.isValidCommittedSealFn = fn
}
