// Package proto defines the code for protocol buffer
package proto

import "google.golang.org/protobuf/proto"

// PayloadNoSig returns marshaled message without signature
func (m *Message) PayloadNoSig() ([]byte, error) {
	mm, _ := proto.Clone(m).(*Message)
	mm.Signature = nil

	raw, err := proto.Marshal(mm)
	if err != nil {
		return nil, err
	}

	return raw, nil
}

// Copy is a helper method for deep copy of Proposal
func (p *Proposal) Copy() *Proposal {
	rawProposal := make([]byte, len(p.RawProposal))

	copy(rawProposal, p.RawProposal)

	return &Proposal{
		RawProposal: rawProposal,
		Round:       p.Round,
	}
}

// Copy is a helper method for deep copy of PreparedCertificate
func (pc *PreparedCertificate) Copy() *PreparedCertificate {
	proposal, _ := proto.Clone(pc.ProposalMessage).(*Message)

	preparedMessages := make([]*Message, len(pc.PrepareMessages))

	for idx, pm := range pc.PrepareMessages {
		prepareMsg, _ := proto.Clone(pm).(*Message)

		preparedMessages[idx] = prepareMsg
	}

	return &PreparedCertificate{
		ProposalMessage: proposal,
		PrepareMessages: preparedMessages,
	}
}
