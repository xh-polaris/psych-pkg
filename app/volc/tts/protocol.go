package tts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

type (
	// EventType defines the event type which determines the event of the message.
	EventType int32
	// MsgType defines message type which determines how the message will be
	// serialized with the protocol.
	MsgType uint8
	// MsgTypeFlagBits defines the 4-bit message-type specific flags. The specific
	// values should be defined in each specific usage scenario.
	MsgTypeFlagBits uint8
	// VersionBits defines the 4-bit version type.
	VersionBits uint8
	// HeaderSizeBits defines the 4-bit header-size type.
	HeaderSizeBits uint8
	// SerializationBits defines the 4-bit serialization method type.
	SerializationBits uint8
	// CompressionBits defines the 4-bit compression method type.
	CompressionBits uint8
)

const (
	MsgTypeFlagNoSeq       MsgTypeFlagBits = 0     // Non-terminal packet with no sequence
	MsgTypeFlagPositiveSeq MsgTypeFlagBits = 0b1   // Non-terminal packet with sequence > 0
	MsgTypeFlagLastNoSeq   MsgTypeFlagBits = 0b10  // last packet with no sequence
	MsgTypeFlagNegativeSeq MsgTypeFlagBits = 0b11  // last packet with sequence < 0
	MsgTypeFlagWithEvent   MsgTypeFlagBits = 0b100 // Payload contains event number (int32)
)

const (
	Version1 VersionBits = iota + 1
	Version2
	Version3
	Version4
)

const (
	HeaderSize4 HeaderSizeBits = iota + 1
	HeaderSize8
	HeaderSize12
	HeaderSize16
)

const (
	SerializationRaw    SerializationBits = 0
	SerializationJSON   SerializationBits = 0b1
	SerializationThrift SerializationBits = 0b11
	SerializationCustom SerializationBits = 0b1111
)

const (
	CompressionNone   CompressionBits = 0
	CompressionGzip   CompressionBits = 0b1
	CompressionCustom CompressionBits = 0b1111
)

const (
	MsgTypeInvalid              MsgType = 0
	MsgTypeFullClientRequest    MsgType = 0b1
	MsgTypeAudioOnlyClient      MsgType = 0b10
	MsgTypeFullServerResponse   MsgType = 0b1001
	MsgTypeAudioOnlyServer      MsgType = 0b1011
	MsgTypeFrontEndResultServer MsgType = 0b1100
	MsgTypeError                MsgType = 0b1111

	MsgTypeServerACK = MsgTypeAudioOnlyServer
)

func (t MsgType) String() string {
	switch t {
	case MsgTypeFullClientRequest:
		return "MsgType_FullClientRequest"
	case MsgTypeAudioOnlyClient:
		return "MsgType_AudioOnlyClient"
	case MsgTypeFullServerResponse:
		return "MsgType_FullServerResponse"
	case MsgTypeAudioOnlyServer:
		return "MsgType_AudioOnlyServer" // MsgTypeServerACK
	case MsgTypeError:
		return "MsgType_Error"
	case MsgTypeFrontEndResultServer:
		return "MsgType_FrontEndResultServer"
	default:
		return fmt.Sprintf("MsgType_(%d)", t)
	}
}

const (
	// Default event, applicable for scenarios not using events or not requiring event transmission,
	// or for scenarios using events, non-zero values can be used to validate event legitimacy
	EventType_None EventType = 0
	// 1 ~ 49 for upstream Connection events
	EventType_StartConnection  EventType = 1
	EventType_StartTask        EventType = 1 // Alias of "StartConnection"
	EventType_FinishConnection EventType = 2
	EventType_FinishTask       EventType = 2 // Alias of "FinishConnection"
	// 50 ~ 99 for downstream Connection events
	// Connection established successfully
	EventType_ConnectionStarted EventType = 50
	EventType_TaskStarted       EventType = 50 // Alias of "ConnectionStarted"
	// Connection failed (possibly due to authentication failure)
	EventType_ConnectionFailed EventType = 51
	EventType_TaskFailed       EventType = 51 // Alias of "ConnectionFailed"
	// Connection ended
	EventType_ConnectionFinished EventType = 52
	EventType_TaskFinished       EventType = 52 // Alias of "ConnectionFinished"
	// 100 ~ 149 for upstream Session events
	EventType_StartSession  EventType = 100
	EventType_CancelSession EventType = 101
	EventType_FinishSession EventType = 102
	// 150 ~ 199 for downstream Session events
	EventType_SessionStarted  EventType = 150
	EventType_SessionCanceled EventType = 151
	EventType_SessionFinished EventType = 152
	EventType_SessionFailed   EventType = 153
	// Usage events
	EventType_UsageResponse EventType = 154
	EventType_ChargeData    EventType = 154 // Alias of "UsageResponse"
	// 200 ~ 249 for upstream general events
	EventType_TaskRequest  EventType = 200
	EventType_UpdateConfig EventType = 201
	// 250 ~ 299 for downstream general events
	EventType_AudioMuted EventType = 250
	// 300 ~ 349 for upstream TTS events
	EventType_SayHello EventType = 300
	// 350 ~ 399 for downstream TTS events
	EventType_TTSSentenceStart     EventType = 350
	EventType_TTSSentenceEnd       EventType = 351
	EventType_TTSResponse          EventType = 352
	EventType_TTSEnded             EventType = 359
	EventType_PodcastRoundStart    EventType = 360
	EventType_PodcastRoundResponse EventType = 361
	EventType_PodcastRoundEnd      EventType = 362
	// 450 ~ 499 for downstream ASR events
	EventType_ASRInfo     EventType = 450
	EventType_ASRResponse EventType = 451
	EventType_ASREnded    EventType = 459
	// 500 ~ 549 for upstream dialogue events
	// (Ground-Truth-Alignment) text for speech synthesis
	EventType_ChatTTSText EventType = 500
	// 550 ~ 599 for downstream dialogue events
	EventType_ChatResponse EventType = 550
	EventType_ChatEnded    EventType = 559
	// 650 ~ 699 for downstream dialogue events
	// Events for source (original) language subtitle.
	EventType_SourceSubtitleStart    EventType = 650
	EventType_SourceSubtitleResponse EventType = 651
	EventType_SourceSubtitleEnd      EventType = 652
	// Events for target (translation) language subtitle.
	EventType_TranslationSubtitleStart    EventType = 653
	EventType_TranslationSubtitleResponse EventType = 654
	EventType_TranslationSubtitleEnd      EventType = 655
)

func (t EventType) String() string {
	switch t {
	case EventType_None:
		return "EventType_None"
	case EventType_StartConnection:
		return "EventType_StartConnection"
	case EventType_FinishConnection:
		return "EventType_FinishConnection"
	case EventType_ConnectionStarted:
		return "EventType_ConnectionStarted"
	case EventType_ConnectionFailed:
		return "EventType_ConnectionFailed"
	case EventType_ConnectionFinished:
		return "EventType_ConnectionFinished"
	case EventType_StartSession:
		return "EventType_StartSession"
	case EventType_CancelSession:
		return "EventType_CancelSession"
	case EventType_FinishSession:
		return "EventType_FinishSession"
	case EventType_SessionStarted:
		return "EventType_SessionStarted"
	case EventType_SessionCanceled:
		return "EventType_SessionCanceled"
	case EventType_SessionFinished:
		return "EventType_SessionFinished"
	case EventType_SessionFailed:
		return "EventType_SessionFailed"
	case EventType_UsageResponse:
		return "EventType_UsageResponse"
	case EventType_TaskRequest:
		return "EventType_TaskRequest"
	case EventType_UpdateConfig:
		return "EventType_UpdateConfig"
	case EventType_AudioMuted:
		return "EventType_AudioMuted"
	case EventType_SayHello:
		return "EventType_SayHello"
	case EventType_TTSSentenceStart:
		return "EventType_TTSSentenceStart"
	case EventType_TTSSentenceEnd:
		return "EventType_TTSSentenceEnd"
	case EventType_TTSResponse:
		return "EventType_TTSResponse"
	case EventType_TTSEnded:
		return "EventType_TTSEnded"
	case EventType_PodcastRoundStart:
		return "EventType_PodcastRoundStart"
	case EventType_PodcastRoundResponse:
		return "EventType_PodcastRoundResponse"
	case EventType_PodcastRoundEnd:
		return "EventType_PodcastRoundEnd"
	case EventType_ASRInfo:
		return "EventType_ASRInfo"
	case EventType_ASRResponse:
		return "EventType_ASRResponse"
	case EventType_ASREnded:
		return "EventType_ASREnded"
	case EventType_ChatTTSText:
		return "EventType_ChatTTSText"
	case EventType_ChatResponse:
		return "EventType_ChatResponse"
	case EventType_ChatEnded:
		return "EventType_ChatEnded"
	case EventType_SourceSubtitleStart:
		return "EventType_SourceSubtitleStart"
	case EventType_SourceSubtitleResponse:
		return "EventType_SourceSubtitleResponse"
	case EventType_SourceSubtitleEnd:
		return "EventType_SourceSubtitleEnd"
	case EventType_TranslationSubtitleStart:
		return "EventType_TranslationSubtitleStart"
	case EventType_TranslationSubtitleResponse:
		return "EventType_TranslationSubtitleResponse"
	case EventType_TranslationSubtitleEnd:
		return "EventType_TranslationSubtitleEnd"
	default:
		return fmt.Sprintf("EventType_(%d)", t)
	}
}

// 0                 1                 2                 3
// | 0 1 2 3 4 5 6 7 | 0 1 2 3 4 5 6 7 | 0 1 2 3 4 5 6 7 | 0 1 2 3 4 5 6 7 |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |    Version      |   Header Size   |     Msg Type    |      Flags      |
// |   (4 bits)      |    (4 bits)     |     (4 bits)    |     (4 bits)    |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// | Serialization   |   Compression   |           Reserved                |
// |   (4 bits)      |    (4 bits)     |           (8 bits)                |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                                                                       |
// |                   Optional Header Extensions                          |
// |                     (if Header Size > 1)                              |
// |                                                                       |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                                                                       |
// |                           Payload                                     |
// |                      (variable length)                                |
// |                                                                       |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

type Message struct {
	Version       VersionBits
	HeaderSize    HeaderSizeBits
	MsgType       MsgType
	MsgTypeFlag   MsgTypeFlagBits
	Serialization SerializationBits
	Compression   CompressionBits

	EventType EventType
	SessionID string
	ConnectID string
	Sequence  int32
	ErrorCode uint32

	Payload []byte
}

func NewMessageFromBytes(data []byte) (*Message, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("data too short: expected at least 3 bytes, got %d", len(data))
	}

	typeAndFlag := data[1]

	msg, err := NewMessage(MsgType(typeAndFlag>>4), MsgTypeFlagBits(typeAndFlag&0b00001111))
	if err != nil {
		return nil, err
	}

	if err := msg.Unmarshal(data); err != nil {
		return nil, err
	}

	return msg, nil
}

func NewMessage(msgType MsgType, flag MsgTypeFlagBits) (*Message, error) {
	return &Message{
		MsgType:       msgType,
		MsgTypeFlag:   flag,
		Version:       Version1,
		HeaderSize:    HeaderSize4,
		Serialization: SerializationJSON,
		Compression:   CompressionNone,
	}, nil
}

func (m *Message) String() string {
	switch m.MsgType {
	case MsgTypeAudioOnlyServer, MsgTypeAudioOnlyClient:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			return fmt.Sprintf("%s, %s, Sequence: %d, PayloadSize: %d", m.MsgType, m.EventType, m.Sequence, len(m.Payload))
		}
		return fmt.Sprintf("%s, %s, PayloadSize: %d", m.MsgType, m.EventType, len(m.Payload))
	case MsgTypeError:
		return fmt.Sprintf("%s, %s, ErrorCode: %d, Payload: %s", m.MsgType, m.EventType, m.ErrorCode, string(m.Payload))
	default:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			return fmt.Sprintf("%s, %s, Sequence: %d, Payload: %s",
				m.MsgType, m.EventType, m.Sequence, string(m.Payload))
		}
		return fmt.Sprintf("%s, %s, Payload: %s", m.MsgType, m.EventType, string(m.Payload))
	}
}

func (m *Message) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	header := []uint8{
		uint8(m.Version)<<4 | uint8(m.HeaderSize),
		uint8(m.MsgType)<<4 | uint8(m.MsgTypeFlag),
		uint8(m.Serialization)<<4 | uint8(m.Compression),
	}

	headerSize := 4 * int(m.HeaderSize)
	if padding := headerSize - len(header); padding > 0 {
		header = append(header, make([]uint8, padding)...)
	}

	if err := binary.Write(buf, binary.BigEndian, header); err != nil {
		return nil, err
	}

	writers, err := m.writers()
	if err != nil {
		return nil, err
	}

	for _, write := range writers {
		if err := write(buf); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (m *Message) Unmarshal(data []byte) error {
	buf := bytes.NewBuffer(data)

	versionAndHeaderSize, err := buf.ReadByte()
	if err != nil {
		return err
	}

	m.Version = VersionBits(versionAndHeaderSize >> 4)
	m.HeaderSize = HeaderSizeBits(versionAndHeaderSize & 0b00001111)

	_, err = buf.ReadByte()
	if err != nil {
		return err
	}

	serializationCompression, err := buf.ReadByte()
	if err != nil {
		return err
	}

	m.Serialization = SerializationBits(serializationCompression & 0b11110000)
	m.Compression = CompressionBits(serializationCompression & 0b00001111)

	headerSize := 4 * int(m.HeaderSize)
	readSize := 3
	if paddingSize := headerSize - readSize; paddingSize > 0 {
		if n, err := buf.Read(make([]byte, paddingSize)); err != nil || n < paddingSize {
			return fmt.Errorf("insufficient header bytes: expected %d, got %d", paddingSize, n)
		}
	}

	readers, err := m.readers()
	if err != nil {
		return err
	}

	for _, read := range readers {
		if err := read(buf); err != nil {
			return err
		}
	}

	if _, err := buf.ReadByte(); err != io.EOF {
		return fmt.Errorf("unexpected data after message: %v", err)
	}

	return nil
}

func (m *Message) writers() (writers []func(*bytes.Buffer) error, _ error) {
	if m.MsgTypeFlag == MsgTypeFlagWithEvent {
		writers = append(writers, m.writeEvent, m.writeSessionID)
	}

	switch m.MsgType {
	case MsgTypeFullClientRequest, MsgTypeFullServerResponse, MsgTypeFrontEndResultServer, MsgTypeAudioOnlyClient, MsgTypeAudioOnlyServer:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			writers = append(writers, m.writeSequence)
		}
	case MsgTypeError:
		writers = append(writers, m.writeErrorCode)
	default:
		return nil, fmt.Errorf("unsupported message type: %d", m.MsgType)
	}

	writers = append(writers, m.writePayload)
	return writers, nil
}

func (m *Message) writeEvent(buf *bytes.Buffer) error {
	return binary.Write(buf, binary.BigEndian, m.EventType)
}

func (m *Message) writeSessionID(buf *bytes.Buffer) error {
	switch m.EventType {
	case EventType_StartConnection, EventType_FinishConnection,
		EventType_ConnectionStarted, EventType_ConnectionFailed:
		return nil
	}

	size := len(m.SessionID)
	if size > math.MaxUint32 {
		return fmt.Errorf("session ID size (%d) exceeds max(uint32)", size)
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(size)); err != nil {
		return err
	}

	buf.WriteString(m.SessionID)
	return nil
}

func (m *Message) writeSequence(buf *bytes.Buffer) error {
	return binary.Write(buf, binary.BigEndian, m.Sequence)
}

func (m *Message) writeErrorCode(buf *bytes.Buffer) error {
	return binary.Write(buf, binary.BigEndian, m.ErrorCode)
}

func (m *Message) writePayload(buf *bytes.Buffer) error {
	size := len(m.Payload)
	if size > math.MaxUint32 {
		return fmt.Errorf("payload size (%d) exceeds max(uint32)", size)
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(size)); err != nil {
		return err
	}

	buf.Write(m.Payload)
	return nil
}

func (m *Message) readers() (readers []func(*bytes.Buffer) error, _ error) {
	switch m.MsgType {
	case MsgTypeFullClientRequest, MsgTypeFullServerResponse, MsgTypeFrontEndResultServer, MsgTypeAudioOnlyClient, MsgTypeAudioOnlyServer:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			readers = append(readers, m.readSequence)
		}
	case MsgTypeError:
		readers = append(readers, m.readErrorCode)
	default:
		return nil, fmt.Errorf("unsupported message type: %d", m.MsgType)
	}

	if m.MsgTypeFlag == MsgTypeFlagWithEvent {
		readers = append(readers, m.readEvent, m.readSessionID, m.readConnectID)
	}

	readers = append(readers, m.readPayload)
	return readers, nil
}

func (m *Message) readEvent(buf *bytes.Buffer) error {
	return binary.Read(buf, binary.BigEndian, &m.EventType)
}

func (m *Message) readSessionID(buf *bytes.Buffer) error {
	switch m.EventType {
	case EventType_StartConnection, EventType_FinishConnection,
		EventType_ConnectionStarted, EventType_ConnectionFailed,
		EventType_ConnectionFinished:
		return nil
	}

	var size uint32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return err
	}

	if size > 0 {
		m.SessionID = string(buf.Next(int(size)))
	}

	return nil
}

func (m *Message) readConnectID(buf *bytes.Buffer) error {
	switch m.EventType {
	case EventType_ConnectionStarted, EventType_ConnectionFailed,
		EventType_ConnectionFinished:
	default:
		return nil
	}

	var size uint32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return err
	}

	if size > 0 {
		m.ConnectID = string(buf.Next(int(size)))
	}

	return nil
}

func (m *Message) readSequence(buf *bytes.Buffer) error {
	return binary.Read(buf, binary.BigEndian, &m.Sequence)
}

func (m *Message) readErrorCode(buf *bytes.Buffer) error {
	return binary.Read(buf, binary.BigEndian, &m.ErrorCode)
}

func (m *Message) readPayload(buf *bytes.Buffer) error {
	var size uint32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return err
	}

	if size > 0 {
		m.Payload = buf.Next(int(size))
	}

	return nil
}

func CancelSession(conn *websocket.Conn, sessionID string) error {
	msg, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagWithEvent)
	if err != nil {
		return err
	}
	msg.EventType = EventType_CancelSession
	msg.SessionID = sessionID
	msg.Payload = []byte("{}")
	glog.Info("send: ", msg)
	frame, err := msg.Marshal()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}

type TTSRequest struct {
	User *TTSUser `protobuf:"bytes,1,opt,name=user,proto3" json:"user,omitempty"`
	// Similar to TTSResponse.event field.
	Event     int32         `protobuf:"varint,2,opt,name=event,proto3" json:"event,omitempty"`
	Namespace string        `protobuf:"bytes,3,opt,name=namespace,proto3" json:"namespace,omitempty"`
	ReqParams *TTSReqParams `protobuf:"bytes,10,opt,name=req_params,json=reqParams,proto3" json:"req_params,omitempty"`
}

type TTSUser struct {
	Uid            string `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	Did            string `protobuf:"bytes,2,opt,name=did,proto3" json:"did,omitempty"`
	DevicePlatform string `protobuf:"bytes,3,opt,name=device_platform,json=devicePlatform,proto3" json:"device_platform,omitempty"`
	DeviceType     string `protobuf:"bytes,4,opt,name=device_type,json=deviceType,proto3" json:"device_type,omitempty"`
	VersionCode    string `protobuf:"bytes,5,opt,name=version_code,json=versionCode,proto3" json:"version_code,omitempty"`
	Language       string `protobuf:"bytes,6,opt,name=language,proto3" json:"language,omitempty"`
}
type Texts struct {
	Texts []string `protobuf:"bytes,1,rep,name=texts,proto3" json:"texts,omitempty"`
}

type AudioParams struct {
	Format          string `protobuf:"bytes,1,opt,name=format,proto3" json:"format,omitempty"`
	SampleRate      int32  `protobuf:"varint,2,opt,name=sample_rate,json=sampleRate,proto3" json:"sample_rate,omitempty"`
	Channel         int32  `protobuf:"varint,3,opt,name=channel,proto3" json:"channel,omitempty"`
	SpeechRate      int32  `protobuf:"varint,4,opt,name=speech_rate,json=speechRate,proto3" json:"speech_rate,omitempty"`
	PitchRate       int32  `protobuf:"varint,5,opt,name=pitch_rate,json=pitchRate,proto3" json:"pitch_rate,omitempty"`
	BitRate         int32  `protobuf:"varint,6,opt,name=bit_rate,json=bitRate,proto3" json:"bit_rate,omitempty"`
	Volume          int32  `protobuf:"varint,7,opt,name=volume,proto3" json:"volume,omitempty"`
	Lang            string `protobuf:"bytes,8,opt,name=lang,proto3" json:"lang,omitempty"`
	Emotion         string `protobuf:"bytes,9,opt,name=emotion,proto3" json:"emotion,omitempty"`
	Gender          string `protobuf:"bytes,10,opt,name=gender,proto3" json:"gender,omitempty"`
	EnableTimestamp bool   `protobuf:"varint,11,opt,name=enable_timestamp,json=enableTimestamp,proto3" json:"enable_timestamp,omitempty"`
}
type EngineParams struct {
	EngineContext                string   `protobuf:"bytes,1,opt,name=engine_context,json=engineContext,proto3" json:"engine_context,omitempty"`
	PhonemeSize                  string   `protobuf:"bytes,2,opt,name=phoneme_size,json=phonemeSize,proto3" json:"phoneme_size,omitempty"`
	EnableFastTextSeg            bool     `protobuf:"varint,3,opt,name=enable_fast_text_seg,json=enableFastTextSeg,proto3" json:"enable_fast_text_seg,omitempty"`
	ForceBreak                   bool     `protobuf:"varint,4,opt,name=force_break,json=forceBreak,proto3" json:"force_break,omitempty"`
	BreakByProsody               int32    `protobuf:"varint,5,opt,name=break_by_prosody,json=breakByProsody,proto3" json:"break_by_prosody,omitempty"`
	EnableEngineDebugInfo        bool     `protobuf:"varint,6,opt,name=enable_engine_debug_info,json=enableEngineDebugInfo,proto3" json:"enable_engine_debug_info,omitempty"`
	FlushSentence                bool     `protobuf:"varint,7,opt,name=flush_sentence,json=flushSentence,proto3" json:"flush_sentence,omitempty"`
	LabVersion                   string   `protobuf:"bytes,8,opt,name=lab_version,json=labVersion,proto3" json:"lab_version,omitempty"`
	EnableIpaExtraction          bool     `protobuf:"varint,9,opt,name=enable_ipa_extraction,json=enableIpaExtraction,proto3" json:"enable_ipa_extraction,omitempty"`
	EnableNaiveTn                bool     `protobuf:"varint,10,opt,name=enable_naive_tn,json=enableNaiveTn,proto3" json:"enable_naive_tn,omitempty"`
	EnableLatexTn                bool     `protobuf:"varint,11,opt,name=enable_latex_tn,json=enableLatexTn,proto3" json:"enable_latex_tn,omitempty"`
	DisableNewlineStrategy       bool     `protobuf:"varint,12,opt,name=disable_newline_strategy,json=disableNewlineStrategy,proto3" json:"disable_newline_strategy,omitempty"`
	SupportedLanguages           []string `protobuf:"bytes,13,rep,name=supported_languages,json=supportedLanguages,proto3" json:"supported_languages,omitempty"`
	ContextLanguage              string   `protobuf:"bytes,14,opt,name=context_language,json=contextLanguage,proto3" json:"context_language,omitempty"`
	ContextTexts                 []string `protobuf:"bytes,15,rep,name=context_texts,json=contextTexts,proto3" json:"context_texts,omitempty"`
	EnableRecoverPuncts          bool     `protobuf:"varint,16,opt,name=enable_recover_puncts,json=enableRecoverPuncts,proto3" json:"enable_recover_puncts,omitempty"`
	EosProsody                   int32    `protobuf:"varint,17,opt,name=eos_prosody,json=eosProsody,proto3" json:"eos_prosody,omitempty"`
	PrependSilenceSeconds        float64  `protobuf:"fixed64,18,opt,name=prepend_silence_seconds,json=prependSilenceSeconds,proto3" json:"prepend_silence_seconds,omitempty"`
	MaxParagraphPhonemeSize      int32    `protobuf:"varint,19,opt,name=max_paragraph_phoneme_size,json=maxParagraphPhonemeSize,proto3" json:"max_paragraph_phoneme_size,omitempty"`
	ParagraphSubSentences        []string `protobuf:"bytes,20,rep,name=paragraph_sub_sentences,json=paragraphSubSentences,proto3" json:"paragraph_sub_sentences,omitempty"`
	MaxLengthToFilterParenthesis int32    `protobuf:"varint,21,opt,name=max_length_to_filter_parenthesis,json=maxLengthToFilterParenthesis,proto3" json:"max_length_to_filter_parenthesis,omitempty"`
	EnableLanguageDetector       bool     `protobuf:"varint,22,opt,name=enable_language_detector,json=enableLanguageDetector,proto3" json:"enable_language_detector,omitempty"`
}

type TTSReqParams struct {
	Text           string        `protobuf:"bytes,1,opt,name=text,proto3" json:"text,omitempty"`
	Texts          *Texts        `protobuf:"bytes,2,opt,name=texts,proto3" json:"texts,omitempty"`
	Ssml           string        `protobuf:"bytes,3,opt,name=ssml,proto3" json:"ssml,omitempty"`
	Speaker        string        `protobuf:"bytes,4,opt,name=speaker,proto3" json:"speaker,omitempty"`
	AudioParams    *AudioParams  `protobuf:"bytes,5,opt,name=audio_params,json=audioParams,proto3" json:"audio_params,omitempty"`
	EngineParams   *EngineParams `protobuf:"bytes,6,opt,name=engine_params,json=engineParams,proto3" json:"engine_params,omitempty"`
	EnableAudio2Bs bool          `protobuf:"varint,7,opt,name=enable_audio2bs,json=enableAudio2bs,proto3" json:"enable_audio2bs,omitempty"`
	EnableTextSeg  bool          `protobuf:"varint,8,opt,name=enable_text_seg,json=enableTextSeg,proto3" json:"enable_text_seg,omitempty"`
	Additions      string        `protobuf:"bytes,100,rep,name=additions,proto3" json:"additions,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}
