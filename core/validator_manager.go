package core

import (
	"bytes"
	"errors"
	"math/big"
	"sync"

	"github.com/0xPolygon/go-ibft/messages/proto"
)

var (
	errVotingPowerNotCorrect = errors.New("total voting power is zero or less")
)

// ValidatorBackend defines interface that has GetVotingPower
type ValidatorBackend interface {
	// GetVotingPowers returns map of validators addresses and their voting powers for the specified height.
	GetVotingPowers(height uint64) (map[string]*big.Int, error)
}

// ValidatorManager keeps voting power and other information about validators
type ValidatorManager struct {
	vpLock *sync.RWMutex

	// quorumSize represents quorum for the height specified in the current View
	quorumSize *big.Int

	// validatorsVotingPower is a map of the validator addresses on their voting power for
	// the height specified in the current View
	validatorsVotingPower map[string]*big.Int

	backend ValidatorBackend

	log Logger
}

// NewValidatorManager creates new ValidatorManager
func NewValidatorManager(backend ValidatorBackend, log Logger) *ValidatorManager {
	return &ValidatorManager{
		quorumSize:            big.NewInt(0),
		backend:               backend,
		validatorsVotingPower: nil,
		log:                   log,
		vpLock:                &sync.RWMutex{},
	}
}

// Init sets voting power and quorum size
func (vm *ValidatorManager) Init(height uint64) error {
	vm.vpLock.Lock()
	defer vm.vpLock.Unlock()

	validatorsVotingPower, err := vm.backend.GetVotingPowers(height)
	if err != nil {
		return err
	}

	totalVotingPower := calculateTotalVotingPower(validatorsVotingPower)
	if totalVotingPower.Cmp(big.NewInt(0)) <= 0 {
		return errVotingPowerNotCorrect
	}

	vm.validatorsVotingPower = validatorsVotingPower
	vm.quorumSize = calculateQuorum(totalVotingPower)

	return nil
}

// HasQuorum provides information on whether messages have reached the quorum
func (vm *ValidatorManager) HasQuorum(sendersAddrs map[string]struct{}) bool {
	vm.vpLock.RLock()
	defer vm.vpLock.RUnlock()

	// if not initialized correctly return false
	if vm.validatorsVotingPower == nil {
		return false
	}

	messageVotePower := big.NewInt(0)

	for from := range sendersAddrs {
		if vote, ok := vm.validatorsVotingPower[from]; ok {
			messageVotePower.Add(messageVotePower, vote)
		}
	}

	return messageVotePower.Cmp(vm.quorumSize) >= 0
}

// HasPrepareQuorum provides information on whether prepared messages have reached the quorum
func (vm *ValidatorManager) HasPrepareQuorum(proposalMessage *proto.Message, msgs []*proto.Message) bool {
	if proposalMessage == nil {
		vm.log.Error("HasPrepareQuorum - proposalMessage is not set")

		return false
	}

	proposerAddress := proposalMessage.From
	sendersAddressesMap := map[string]struct{}{
		string(proposerAddress): {},
	}

	for _, message := range msgs {
		if bytes.Equal(message.From, proposerAddress) {
			vm.log.Error("HasPrepareQuorum - proposer is among signers but it is not expected to be")

			return false
		}

		sendersAddressesMap[string(message.From)] = struct{}{}
	}

	return vm.HasQuorum(sendersAddressesMap)
}

func calculateQuorum(totalVotingPower *big.Int) *big.Int {
	quorum := new(big.Int).Mul(totalVotingPower, big.NewInt(2))

	return bigIntDivCeil(quorum, big.NewInt(3))
}

func calculateTotalVotingPower(validatorsVotingPower map[string]*big.Int) *big.Int {
	totalVotingPower := big.NewInt(0)
	for _, validatorVotingPower := range validatorsVotingPower {
		totalVotingPower.Add(totalVotingPower, validatorVotingPower)
	}

	return totalVotingPower
}

// bigIntDivCeil performs integer division and rounds given result to next bigger integer number
// It is calculated using this formula result = (a + b - 1) / b
func bigIntDivCeil(a, b *big.Int) *big.Int {
	result := new(big.Int)

	return result.Add(a, b).
		Sub(result, big.NewInt(1)).
		Div(result, b)
}

// convertMessageToAddressSet converts messages slice to addresses map
func convertMessageToAddressSet(messages []*proto.Message) map[string]struct{} {
	result := make(map[string]struct{}, len(messages))

	for _, x := range messages {
		result[string(x.From)] = struct{}{}
	}

	return result
}
