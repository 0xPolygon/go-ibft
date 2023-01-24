// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.12.4
// source: messages/proto/messages.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// MessageType defines the types of messages
// circulating in the system
type MessageType int32

const (
	MessageType_PREPREPARE   MessageType = 0
	MessageType_PREPARE      MessageType = 1
	MessageType_COMMIT       MessageType = 2
	MessageType_ROUND_CHANGE MessageType = 3
)

// Enum value maps for MessageType.
var (
	MessageType_name = map[int32]string{
		0: "PREPREPARE",
		1: "PREPARE",
		2: "COMMIT",
		3: "ROUND_CHANGE",
	}
	MessageType_value = map[string]int32{
		"PREPREPARE":   0,
		"PREPARE":      1,
		"COMMIT":       2,
		"ROUND_CHANGE": 3,
	}
)

func (x MessageType) Enum() *MessageType {
	p := new(MessageType)
	*p = x
	return p
}

func (x MessageType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (MessageType) Descriptor() protoreflect.EnumDescriptor {
	return file_messages_proto_messages_proto_enumTypes[0].Descriptor()
}

func (MessageType) Type() protoreflect.EnumType {
	return &file_messages_proto_messages_proto_enumTypes[0]
}

func (x MessageType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use MessageType.Descriptor instead.
func (MessageType) EnumDescriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{0}
}

// View defines the current status
type View struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// height represents the number of the proposal
	Height uint64 `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	// round represents the round number in the specific height
	Round uint64 `protobuf:"varint,2,opt,name=round,proto3" json:"round,omitempty"`
}

func (x *View) Reset() {
	*x = View{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *View) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*View) ProtoMessage() {}

func (x *View) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use View.ProtoReflect.Descriptor instead.
func (*View) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{0}
}

func (x *View) GetHeight() uint64 {
	if x != nil {
		return x.Height
	}
	return 0
}

func (x *View) GetRound() uint64 {
	if x != nil {
		return x.Round
	}
	return 0
}

// Message defines the base message structure
type Message struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// view is the current view for the message
	View *View `protobuf:"bytes,1,opt,name=view,proto3" json:"view,omitempty"`
	// from defines who is the message sender
	From []byte `protobuf:"bytes,2,opt,name=from,proto3" json:"from,omitempty"`
	// the signature of the sender, if any
	Signature []byte `protobuf:"bytes,3,opt,name=signature,proto3" json:"signature,omitempty"`
	// type defines the message type
	Type MessageType `protobuf:"varint,4,opt,name=type,proto3,enum=MessageType" json:"type,omitempty"`
	// payload is the specific message payload
	//
	// Types that are assignable to Payload:
	//	*Message_PreprepareData
	//	*Message_PrepareData
	//	*Message_CommitData
	//	*Message_RoundChangeData
	Payload isMessage_Payload `protobuf_oneof:"payload"`
}

func (x *Message) Reset() {
	*x = Message{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Message) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Message) ProtoMessage() {}

func (x *Message) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Message.ProtoReflect.Descriptor instead.
func (*Message) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{1}
}

func (x *Message) GetView() *View {
	if x != nil {
		return x.View
	}
	return nil
}

func (x *Message) GetFrom() []byte {
	if x != nil {
		return x.From
	}
	return nil
}

func (x *Message) GetSignature() []byte {
	if x != nil {
		return x.Signature
	}
	return nil
}

func (x *Message) GetType() MessageType {
	if x != nil {
		return x.Type
	}
	return MessageType_PREPREPARE
}

func (m *Message) GetPayload() isMessage_Payload {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (x *Message) GetPreprepareData() *PrePrepareMessage {
	if x, ok := x.GetPayload().(*Message_PreprepareData); ok {
		return x.PreprepareData
	}
	return nil
}

func (x *Message) GetPrepareData() *PrepareMessage {
	if x, ok := x.GetPayload().(*Message_PrepareData); ok {
		return x.PrepareData
	}
	return nil
}

func (x *Message) GetCommitData() *CommitMessage {
	if x, ok := x.GetPayload().(*Message_CommitData); ok {
		return x.CommitData
	}
	return nil
}

func (x *Message) GetRoundChangeData() *RoundChangeMessage {
	if x, ok := x.GetPayload().(*Message_RoundChangeData); ok {
		return x.RoundChangeData
	}
	return nil
}

type isMessage_Payload interface {
	isMessage_Payload()
}

type Message_PreprepareData struct {
	PreprepareData *PrePrepareMessage `protobuf:"bytes,5,opt,name=preprepareData,proto3,oneof"`
}

type Message_PrepareData struct {
	PrepareData *PrepareMessage `protobuf:"bytes,6,opt,name=prepareData,proto3,oneof"`
}

type Message_CommitData struct {
	CommitData *CommitMessage `protobuf:"bytes,7,opt,name=commitData,proto3,oneof"`
}

type Message_RoundChangeData struct {
	RoundChangeData *RoundChangeMessage `protobuf:"bytes,8,opt,name=roundChangeData,proto3,oneof"`
}

func (*Message_PreprepareData) isMessage_Payload() {}

func (*Message_PrepareData) isMessage_Payload() {}

func (*Message_CommitData) isMessage_Payload() {}

func (*Message_RoundChangeData) isMessage_Payload() {}

// PrePrepareMessage is the message for the PREPREPARE phase
type PrePrepareMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// proposal is the actual data being proposed for consensus
	Proposal *Proposal `protobuf:"bytes,1,opt,name=proposal,proto3" json:"proposal,omitempty"`
	// proposalHash is the Keccak hash of the proposal
	ProposalHash []byte `protobuf:"bytes,2,opt,name=proposalHash,proto3" json:"proposalHash,omitempty"`
	// certificate is the RCC that can accompany
	// a proposal message
	Certificate *RoundChangeCertificate `protobuf:"bytes,3,opt,name=certificate,proto3" json:"certificate,omitempty"`
}

func (x *PrePrepareMessage) Reset() {
	*x = PrePrepareMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PrePrepareMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PrePrepareMessage) ProtoMessage() {}

func (x *PrePrepareMessage) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PrePrepareMessage.ProtoReflect.Descriptor instead.
func (*PrePrepareMessage) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{2}
}

func (x *PrePrepareMessage) GetProposal() *Proposal {
	if x != nil {
		return x.Proposal
	}
	return nil
}

func (x *PrePrepareMessage) GetProposalHash() []byte {
	if x != nil {
		return x.ProposalHash
	}
	return nil
}

func (x *PrePrepareMessage) GetCertificate() *RoundChangeCertificate {
	if x != nil {
		return x.Certificate
	}
	return nil
}

// PrepareMessage is the message for the PREPARE phase
type PrepareMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// proposalHash is the Keccak hash of the proposal
	ProposalHash []byte `protobuf:"bytes,1,opt,name=proposalHash,proto3" json:"proposalHash,omitempty"`
}

func (x *PrepareMessage) Reset() {
	*x = PrepareMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PrepareMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PrepareMessage) ProtoMessage() {}

func (x *PrepareMessage) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PrepareMessage.ProtoReflect.Descriptor instead.
func (*PrepareMessage) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{3}
}

func (x *PrepareMessage) GetProposalHash() []byte {
	if x != nil {
		return x.ProposalHash
	}
	return nil
}

// CommitMessage is the message for the COMMIT phase
type CommitMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// proposalHash is the Keccak hash of the proposal
	ProposalHash []byte `protobuf:"bytes,1,opt,name=proposalHash,proto3" json:"proposalHash,omitempty"`
	// committedSeal is the seal of the sender
	CommittedSeal []byte `protobuf:"bytes,2,opt,name=committedSeal,proto3" json:"committedSeal,omitempty"`
}

func (x *CommitMessage) Reset() {
	*x = CommitMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CommitMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CommitMessage) ProtoMessage() {}

func (x *CommitMessage) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CommitMessage.ProtoReflect.Descriptor instead.
func (*CommitMessage) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{4}
}

func (x *CommitMessage) GetProposalHash() []byte {
	if x != nil {
		return x.ProposalHash
	}
	return nil
}

func (x *CommitMessage) GetCommittedSeal() []byte {
	if x != nil {
		return x.CommittedSeal
	}
	return nil
}

// RoundChangeMessage is the message for the ROUND CHANGE phase
type RoundChangeMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// lastProposal is the last proposal
	// to reach Q(N) - 1 PREPARE messages
	LastPreparedProposal *Proposal `protobuf:"bytes,1,opt,name=lastPreparedProposal,proto3" json:"lastPreparedProposal,omitempty"`
	// latestPreparedCertificate is the PC that accompanies
	// the last proposal
	LatestPreparedCertificate *PreparedCertificate `protobuf:"bytes,2,opt,name=latestPreparedCertificate,proto3" json:"latestPreparedCertificate,omitempty"`
}

func (x *RoundChangeMessage) Reset() {
	*x = RoundChangeMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RoundChangeMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RoundChangeMessage) ProtoMessage() {}

func (x *RoundChangeMessage) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RoundChangeMessage.ProtoReflect.Descriptor instead.
func (*RoundChangeMessage) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{5}
}

func (x *RoundChangeMessage) GetLastPreparedProposal() *Proposal {
	if x != nil {
		return x.LastPreparedProposal
	}
	return nil
}

func (x *RoundChangeMessage) GetLatestPreparedCertificate() *PreparedCertificate {
	if x != nil {
		return x.LatestPreparedCertificate
	}
	return nil
}

// PreparedCertificate is a collection of
// prepare messages for a certain proposal
type PreparedCertificate struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// proposalMessage is the proposal message to reach
	// at least Q(N) - 1 PREPARE messages
	ProposalMessage *Message `protobuf:"bytes,1,opt,name=proposalMessage,proto3" json:"proposalMessage,omitempty"`
	// prepareMessages are the PREPARE messages at least Q(N) - 1
	PrepareMessages []*Message `protobuf:"bytes,2,rep,name=prepareMessages,proto3" json:"prepareMessages,omitempty"`
}

func (x *PreparedCertificate) Reset() {
	*x = PreparedCertificate{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PreparedCertificate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PreparedCertificate) ProtoMessage() {}

func (x *PreparedCertificate) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PreparedCertificate.ProtoReflect.Descriptor instead.
func (*PreparedCertificate) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{6}
}

func (x *PreparedCertificate) GetProposalMessage() *Message {
	if x != nil {
		return x.ProposalMessage
	}
	return nil
}

func (x *PreparedCertificate) GetPrepareMessages() []*Message {
	if x != nil {
		return x.PrepareMessages
	}
	return nil
}

// RoundChangeCertificate is a collection of
// round change messages for a certain round
type RoundChangeCertificate struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// roundChangeMessages are the ROUND CHANGE messages
	RoundChangeMessages []*Message `protobuf:"bytes,1,rep,name=roundChangeMessages,proto3" json:"roundChangeMessages,omitempty"`
}

func (x *RoundChangeCertificate) Reset() {
	*x = RoundChangeCertificate{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RoundChangeCertificate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RoundChangeCertificate) ProtoMessage() {}

func (x *RoundChangeCertificate) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RoundChangeCertificate.ProtoReflect.Descriptor instead.
func (*RoundChangeCertificate) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{7}
}

func (x *RoundChangeCertificate) GetRoundChangeMessages() []*Message {
	if x != nil {
		return x.RoundChangeMessages
	}
	return nil
}

// Proposal is the tuple (raw_proposal, round)
type Proposal struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	RawProposal []byte `protobuf:"bytes,1,opt,name=rawProposal,proto3" json:"rawProposal,omitempty"`
	Round       uint64 `protobuf:"varint,2,opt,name=round,proto3" json:"round,omitempty"`
}

func (x *Proposal) Reset() {
	*x = Proposal{}
	if protoimpl.UnsafeEnabled {
		mi := &file_messages_proto_messages_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Proposal) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Proposal) ProtoMessage() {}

func (x *Proposal) ProtoReflect() protoreflect.Message {
	mi := &file_messages_proto_messages_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Proposal.ProtoReflect.Descriptor instead.
func (*Proposal) Descriptor() ([]byte, []int) {
	return file_messages_proto_messages_proto_rawDescGZIP(), []int{8}
}

func (x *Proposal) GetRawProposal() []byte {
	if x != nil {
		return x.RawProposal
	}
	return nil
}

func (x *Proposal) GetRound() uint64 {
	if x != nil {
		return x.Round
	}
	return 0
}

var File_messages_proto_messages_proto protoreflect.FileDescriptor

var file_messages_proto_messages_proto_rawDesc = []byte{
	0x0a, 0x1d, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x34, 0x0a, 0x04, 0x56, 0x69, 0x65, 0x77, 0x12, 0x16, 0x0a, 0x06, 0x68, 0x65, 0x69, 0x67, 0x68,
	0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x06, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74, 0x12,
	0x14, 0x0a, 0x05, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x05,
	0x72, 0x6f, 0x75, 0x6e, 0x64, 0x22, 0xe9, 0x02, 0x0a, 0x07, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x12, 0x19, 0x0a, 0x04, 0x76, 0x69, 0x65, 0x77, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x05, 0x2e, 0x56, 0x69, 0x65, 0x77, 0x52, 0x04, 0x76, 0x69, 0x65, 0x77, 0x12, 0x12, 0x0a, 0x04,
	0x66, 0x72, 0x6f, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x66, 0x72, 0x6f, 0x6d,
	0x12, 0x1c, 0x0a, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x12, 0x20,
	0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x0c, 0x2e, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65,
	0x12, 0x3c, 0x0a, 0x0e, 0x70, 0x72, 0x65, 0x70, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x44, 0x61,
	0x74, 0x61, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x50, 0x72, 0x65, 0x50, 0x72,
	0x65, 0x70, 0x61, 0x72, 0x65, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x48, 0x00, 0x52, 0x0e,
	0x70, 0x72, 0x65, 0x70, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x44, 0x61, 0x74, 0x61, 0x12, 0x33,
	0x0a, 0x0b, 0x70, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x44, 0x61, 0x74, 0x61, 0x18, 0x06, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x48, 0x00, 0x52, 0x0b, 0x70, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x44,
	0x61, 0x74, 0x61, 0x12, 0x30, 0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x44, 0x61, 0x74,
	0x61, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74,
	0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x48, 0x00, 0x52, 0x0a, 0x63, 0x6f, 0x6d, 0x6d, 0x69,
	0x74, 0x44, 0x61, 0x74, 0x61, 0x12, 0x3f, 0x0a, 0x0f, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x43, 0x68,
	0x61, 0x6e, 0x67, 0x65, 0x44, 0x61, 0x74, 0x61, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x13,
	0x2e, 0x52, 0x6f, 0x75, 0x6e, 0x64, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x4d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x48, 0x00, 0x52, 0x0f, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x43, 0x68, 0x61, 0x6e,
	0x67, 0x65, 0x44, 0x61, 0x74, 0x61, 0x42, 0x09, 0x0a, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61,
	0x64, 0x22, 0x99, 0x01, 0x0a, 0x11, 0x50, 0x72, 0x65, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65,
	0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x25, 0x0a, 0x08, 0x70, 0x72, 0x6f, 0x70, 0x6f,
	0x73, 0x61, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x50, 0x72, 0x6f, 0x70,
	0x6f, 0x73, 0x61, 0x6c, 0x52, 0x08, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x12, 0x22,
	0x0a, 0x0c, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x48, 0x61, 0x73, 0x68, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x0c, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x48, 0x61,
	0x73, 0x68, 0x12, 0x39, 0x0a, 0x0b, 0x63, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x52, 0x6f, 0x75, 0x6e, 0x64, 0x43,
	0x68, 0x61, 0x6e, 0x67, 0x65, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65,
	0x52, 0x0b, 0x63, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x22, 0x34, 0x0a,
	0x0e, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12,
	0x22, 0x0a, 0x0c, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x48, 0x61, 0x73, 0x68, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0c, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x48,
	0x61, 0x73, 0x68, 0x22, 0x59, 0x0a, 0x0d, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x12, 0x22, 0x0a, 0x0c, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c,
	0x48, 0x61, 0x73, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0c, 0x70, 0x72, 0x6f, 0x70,
	0x6f, 0x73, 0x61, 0x6c, 0x48, 0x61, 0x73, 0x68, 0x12, 0x24, 0x0a, 0x0d, 0x63, 0x6f, 0x6d, 0x6d,
	0x69, 0x74, 0x74, 0x65, 0x64, 0x53, 0x65, 0x61, 0x6c, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x0d, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x74, 0x65, 0x64, 0x53, 0x65, 0x61, 0x6c, 0x22, 0xa7,
	0x01, 0x0a, 0x12, 0x52, 0x6f, 0x75, 0x6e, 0x64, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x4d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x3d, 0x0a, 0x14, 0x6c, 0x61, 0x73, 0x74, 0x50, 0x72, 0x65,
	0x70, 0x61, 0x72, 0x65, 0x64, 0x50, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x50, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x52, 0x14,
	0x6c, 0x61, 0x73, 0x74, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x64, 0x50, 0x72, 0x6f, 0x70,
	0x6f, 0x73, 0x61, 0x6c, 0x12, 0x52, 0x0a, 0x19, 0x6c, 0x61, 0x74, 0x65, 0x73, 0x74, 0x50, 0x72,
	0x65, 0x70, 0x61, 0x72, 0x65, 0x64, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72,
	0x65, 0x64, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x19, 0x6c,
	0x61, 0x74, 0x65, 0x73, 0x74, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x64, 0x43, 0x65, 0x72,
	0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x22, 0x7d, 0x0a, 0x13, 0x50, 0x72, 0x65, 0x70,
	0x61, 0x72, 0x65, 0x64, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x12,
	0x32, 0x0a, 0x0f, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x4d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x08, 0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x52, 0x0f, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x4d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x12, 0x32, 0x0a, 0x0f, 0x70, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x4d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x08, 0x2e, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x52, 0x0f, 0x70, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x22, 0x54, 0x0a, 0x16, 0x52, 0x6f, 0x75, 0x6e, 0x64,
	0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x65, 0x12, 0x3a, 0x0a, 0x13, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65,
	0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x08,
	0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x52, 0x13, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x43,
	0x68, 0x61, 0x6e, 0x67, 0x65, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x22, 0x42, 0x0a,
	0x08, 0x50, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x12, 0x20, 0x0a, 0x0b, 0x72, 0x61, 0x77,
	0x50, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b,
	0x72, 0x61, 0x77, 0x50, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x12, 0x14, 0x0a, 0x05, 0x72,
	0x6f, 0x75, 0x6e, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x05, 0x72, 0x6f, 0x75, 0x6e,
	0x64, 0x2a, 0x48, 0x0a, 0x0b, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x54, 0x79, 0x70, 0x65,
	0x12, 0x0e, 0x0a, 0x0a, 0x50, 0x52, 0x45, 0x50, 0x52, 0x45, 0x50, 0x41, 0x52, 0x45, 0x10, 0x00,
	0x12, 0x0b, 0x0a, 0x07, 0x50, 0x52, 0x45, 0x50, 0x41, 0x52, 0x45, 0x10, 0x01, 0x12, 0x0a, 0x0a,
	0x06, 0x43, 0x4f, 0x4d, 0x4d, 0x49, 0x54, 0x10, 0x02, 0x12, 0x10, 0x0a, 0x0c, 0x52, 0x4f, 0x55,
	0x4e, 0x44, 0x5f, 0x43, 0x48, 0x41, 0x4e, 0x47, 0x45, 0x10, 0x03, 0x42, 0x11, 0x5a, 0x0f, 0x2f,
	0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_messages_proto_messages_proto_rawDescOnce sync.Once
	file_messages_proto_messages_proto_rawDescData = file_messages_proto_messages_proto_rawDesc
)

func file_messages_proto_messages_proto_rawDescGZIP() []byte {
	file_messages_proto_messages_proto_rawDescOnce.Do(func() {
		file_messages_proto_messages_proto_rawDescData = protoimpl.X.CompressGZIP(file_messages_proto_messages_proto_rawDescData)
	})
	return file_messages_proto_messages_proto_rawDescData
}

var file_messages_proto_messages_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_messages_proto_messages_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_messages_proto_messages_proto_goTypes = []interface{}{
	(MessageType)(0),               // 0: MessageType
	(*View)(nil),                   // 1: View
	(*Message)(nil),                // 2: Message
	(*PrePrepareMessage)(nil),      // 3: PrePrepareMessage
	(*PrepareMessage)(nil),         // 4: PrepareMessage
	(*CommitMessage)(nil),          // 5: CommitMessage
	(*RoundChangeMessage)(nil),     // 6: RoundChangeMessage
	(*PreparedCertificate)(nil),    // 7: PreparedCertificate
	(*RoundChangeCertificate)(nil), // 8: RoundChangeCertificate
	(*Proposal)(nil),               // 9: Proposal
}
var file_messages_proto_messages_proto_depIdxs = []int32{
	1,  // 0: Message.view:type_name -> View
	0,  // 1: Message.type:type_name -> MessageType
	3,  // 2: Message.preprepareData:type_name -> PrePrepareMessage
	4,  // 3: Message.prepareData:type_name -> PrepareMessage
	5,  // 4: Message.commitData:type_name -> CommitMessage
	6,  // 5: Message.roundChangeData:type_name -> RoundChangeMessage
	9,  // 6: PrePrepareMessage.proposal:type_name -> Proposal
	8,  // 7: PrePrepareMessage.certificate:type_name -> RoundChangeCertificate
	9,  // 8: RoundChangeMessage.lastPreparedProposal:type_name -> Proposal
	7,  // 9: RoundChangeMessage.latestPreparedCertificate:type_name -> PreparedCertificate
	2,  // 10: PreparedCertificate.proposalMessage:type_name -> Message
	2,  // 11: PreparedCertificate.prepareMessages:type_name -> Message
	2,  // 12: RoundChangeCertificate.roundChangeMessages:type_name -> Message
	13, // [13:13] is the sub-list for method output_type
	13, // [13:13] is the sub-list for method input_type
	13, // [13:13] is the sub-list for extension type_name
	13, // [13:13] is the sub-list for extension extendee
	0,  // [0:13] is the sub-list for field type_name
}

func init() { file_messages_proto_messages_proto_init() }
func file_messages_proto_messages_proto_init() {
	if File_messages_proto_messages_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_messages_proto_messages_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*View); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Message); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PrePrepareMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PrepareMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CommitMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RoundChangeMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PreparedCertificate); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RoundChangeCertificate); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_messages_proto_messages_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Proposal); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_messages_proto_messages_proto_msgTypes[1].OneofWrappers = []interface{}{
		(*Message_PreprepareData)(nil),
		(*Message_PrepareData)(nil),
		(*Message_CommitData)(nil),
		(*Message_RoundChangeData)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_messages_proto_messages_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_messages_proto_messages_proto_goTypes,
		DependencyIndexes: file_messages_proto_messages_proto_depIdxs,
		EnumInfos:         file_messages_proto_messages_proto_enumTypes,
		MessageInfos:      file_messages_proto_messages_proto_msgTypes,
	}.Build()
	File_messages_proto_messages_proto = out.File
	file_messages_proto_messages_proto_rawDesc = nil
	file_messages_proto_messages_proto_goTypes = nil
	file_messages_proto_messages_proto_depIdxs = nil
}
