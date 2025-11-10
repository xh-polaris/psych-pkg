package asr

import (
	"bytes"
	"encoding/binary"
	"encoding/json"

	"github.com/xh-polaris/psych-pkg/util"
)

// 协议元数据类型与常量

type ProtocolVersion byte
type MessageType byte
type MessageTypeSpecificFlags byte
type SerializationType byte
type CompressionType byte

const (
	Version1 = ProtocolVersion(0b0001) // Version1 协议版本

	ClientFullRequest      = MessageType(0b0001) // 消息类型
	ClientAudioOnlyRequest = MessageType(0b0010)
	ServerFullResponse     = MessageType(0b1001)
	ServerErrorResponse    = MessageType(0b1111)

	NoSequence      = MessageTypeSpecificFlags(0b0000) // no check sequence 	//消息类型特殊标识
	PosSequence     = MessageTypeSpecificFlags(0b0001)
	NegSequence     = MessageTypeSpecificFlags(0b0010)
	NegWithSequence = MessageTypeSpecificFlags(0b0011)

	NoSerialization = SerializationType(0b0000) // 序列化方法
	JSON            = SerializationType(0b0001)

	GZIP = CompressionType(0b0001) // 消息压缩方法
)

var (
	PosDefaultHeader      = DefaultHeader().WithMessageTypeSpecificFlags(PosSequence).toBytes()
	AudioPosDefaultHeader = DefaultHeader().WithMessageType(ClientAudioOnlyRequest).WithMessageTypeSpecificFlags(PosSequence).toBytes()
	AudioNegDefaultHeader = DefaultHeader().WithMessageType(ClientAudioOnlyRequest).WithMessageTypeSpecificFlags(NegWithSequence).toBytes()
)

// Header ASR消息请求头
type Header struct {
	messageType              MessageType
	messageTypeSpecificFlags MessageTypeSpecificFlags
	serializationType        SerializationType
	compressionType          CompressionType
	reservedData             []byte
}

// toBytes转换为消息需要的二进制
func (h *Header) toBytes() []byte {
	header := bytes.NewBuffer([]byte{})
	header.WriteByte(byte(Version1<<4 | 1))
	header.WriteByte(byte(h.messageType<<4) | byte(h.messageTypeSpecificFlags))
	header.WriteByte(byte(h.serializationType<<4) | byte(h.compressionType))
	header.Write(h.reservedData)
	return header.Bytes()
}

func (h *Header) WithMessageType(messageType MessageType) *Header {
	h.messageType = messageType
	return h
}

func (h *Header) WithMessageTypeSpecificFlags(messageTypeSpecificFlags MessageTypeSpecificFlags) *Header {
	h.messageTypeSpecificFlags = messageTypeSpecificFlags
	return h
}

func (h *Header) WithSerializationType(serializationType SerializationType) *Header {
	h.serializationType = serializationType
	return h
}

func (h *Header) WithCompressionType(compressionType CompressionType) *Header {
	h.compressionType = compressionType
	return h
}

func (h *Header) WithReservedData(reservedData []byte) *Header {
	h.reservedData = reservedData
	return h
}

// DefaultHeader 生成默认的Header
func DefaultHeader() *Header {
	return &Header{
		messageType:              ClientFullRequest,
		messageTypeSpecificFlags: PosSequence,
		serializationType:        JSON,
		compressionType:          GZIP,
		reservedData:             []byte{0x00},
	}
}

// UserMeta 用户数据
type UserMeta struct {
	Uid        string `json:"uid,omitempty"`
	Did        string `json:"did,omitempty"`
	Platform   string `json:"platform,omitempty" `
	SDKVersion string `json:"sdk_version,omitempty"`
	APPVersion string `json:"app_version,omitempty"`
}

// AudioMeta 音频数据
type AudioMeta struct {
	Format  string `json:"format,omitempty"`
	Codec   string `json:"codec,omitempty"`
	Rate    int    `json:"rate,omitempty"`
	Bits    int    `json:"bits,omitempty"`
	Channel int    `json:"channel,omitempty"`
}

type CorpusMeta struct {
	BoostingTableName string `json:"boosting_table_name,omitempty"`
	CorrectTableName  string `json:"correct_table_name,omitempty"`
	Context           string `json:"context,omitempty"`
}

// RequestMeta 请求元数据
type RequestMeta struct {
	ModelName       string     `json:"model_name,omitempty"`
	EnableITN       bool       `json:"enable_itn,omitempty"`
	EnablePUNC      bool       `json:"enable_punc,omitempty"`
	EnableDDC       bool       `json:"enable_ddc,omitempty"`
	ShowUtterances  bool       `json:"show_utterances"`
	EnableNonstream bool       `json:"enable_nonstream"`
	ResultType      string     `json:"result_type,omitempty"`
	Corpus          CorpusMeta `json:"corpus,omitempty"`
}

// RequestPayload 请求负载
type RequestPayload struct {
	User    UserMeta    `json:"user"`
	Audio   AudioMeta   `json:"audio"`
	Request RequestMeta `json:"request"`
}

// NewFullClientRequest 客户端请求
func NewFullClientRequest(uid, format, codec string, rate, bits, channel int, name string, itn, punc, ddc bool, resultType string, utterances, stream bool) *RequestPayload {
	return &RequestPayload{
		User: UserMeta{
			Uid: uid,
		},
		Audio: AudioMeta{
			Format:  format,
			Codec:   codec,
			Rate:    rate,
			Bits:    bits,
			Channel: channel,
		},
		Request: RequestMeta{
			ModelName:       name,
			EnableITN:       itn,
			EnablePUNC:      punc,
			EnableDDC:       ddc,
			ShowUtterances:  utterances,
			EnableNonstream: stream,
			ResultType:      resultType,
		},
	}
}

// NewAudioOnlyRequest 音频请求
func NewAudioOnlyRequest(seq int, segment []byte) []byte {
	var request bytes.Buffer

	// write seq
	_ = binary.Write(&request, binary.BigEndian, int32(seq))
	// write payload size
	payload, _ := util.GzipCompress(segment)
	_ = binary.Write(&request, binary.BigEndian, int32(len(payload)))
	// write payload
	request.Write(payload)
	return request.Bytes()
}

// ResponsePayload 响应负载
type ResponsePayload struct {
	AudioInfo struct {
		Duration int `json:"duration"`
	} `json:"audio_info"`
	Result struct {
		Text       string `json:"text"`
		Utterances []struct {
			Definite  bool   `json:"definite"`
			EndTime   int    `json:"end_time"`
			StartTime int    `json:"start_time"`
			Text      string `json:"text"`
			Words     []struct {
				EndTime   int    `json:"end_time"`
				StartTime int    `json:"start_time"`
				Text      string `json:"text"`
			} `json:"words"`
		} `json:"utterances,omitempty"`
	} `json:"result"`
	Error string `json:"error,omitempty"`
}

// Response ASR响应
type Response struct {
	Code            int              `json:"code"`
	Event           int              `json:"event"`
	IsLastPackage   bool             `json:"is_last_package"`
	PayloadSequence int32            `json:"payload_sequence"`
	PayloadSize     int              `json:"payload_size"`
	PayloadMsg      *ResponsePayload `json:"payload_msg"`
}

// ParseResponse 解析响应
func ParseResponse(msg []byte) *Response {
	var result Response

	headerSize := msg[0] & 0x0f                                         // 请求头大小
	messageType := MessageType(msg[1] >> 4)                             // 消息类型
	messageTypeSpecificFlags := MessageTypeSpecificFlags(msg[1] & 0x0f) // 消息类型标识符
	serializationMethod := SerializationType(msg[2] >> 4)               // 序列化方式
	messageCompression := CompressionType(msg[2] & 0x0f)                // 压缩方式
	payload := msg[headerSize*4:]                                       // 有效负载

	// 解析messageTypeSpecificFlags
	if messageTypeSpecificFlags&0x01 != 0 { // 音频消息
		result.PayloadSequence = int32(binary.BigEndian.Uint32(payload[:4]))
		payload = payload[4:]
	}
	if messageTypeSpecificFlags&0x02 != 0 { // 最后一个包
		result.IsLastPackage = true
	}
	if messageTypeSpecificFlags&0x04 != 0 { // 事件消息
		result.Event = int(binary.BigEndian.Uint32(payload[:4]))
		payload = payload[4:]
	}

	// 解析messageType
	switch messageType {
	case ServerFullResponse:
		result.PayloadSize = int(binary.BigEndian.Uint32(payload[:4]))
		payload = payload[4:]
	case ServerErrorResponse:
		result.Code = int(binary.BigEndian.Uint32(payload[:4]))
		result.PayloadSize = int(binary.BigEndian.Uint32(payload[4:8]))
		payload = payload[8:]
	}

	if len(payload) == 0 {
		return &result
	}
	// 是否压缩
	if messageCompression == GZIP {
		payload, _ = util.GzipDecompress(payload)
	}

	// 解析payload
	var asrResponse ResponsePayload
	switch serializationMethod {
	case JSON:
		_ = json.Unmarshal(payload, &asrResponse)
	case NoSerialization:
	}
	result.PayloadMsg = &asrResponse
	return &result
}
