package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xh-polaris/psych-pkg/util"
	"time"
)

// 定义前后端通信的协议

var unimplement = errors.New("unimplement")

var (
	MErr    MType = -1 // 错误消息
	MMeta   MType = 0  // 协议元数据
	MAuth   MType = 1  // 认证消息
	MConfig MType = 2  // 配置消息
	MCmd    MType = 3  // 常规命令
	MResp   MType = 4  // 响应
)

var (
	CUserAudio CType = 1 // 用户音频输入
	CUserText  CType = 2 // 用户文字输入
)

var (
	GZIP int8 = 1
)

var (
	JSON int8 = 1
)

type (
	// MType 消息类型
	MType int8
	// CType 命令类型
	CType int8

	// Message 单条消息
	Message struct {
		Type      MType     `json:"type"`      // 消息类型
		Payload   []byte    `json:"payload"`   // 消息负载
		Timestamp time.Time `json:"timestamp"` // 消息时间戳
	}

	// Meta 元数据
	Meta struct {
		Version       int8 `json:"version"`       // 协议版本
		Serialization int8 `json:"serialization"` // 序列化方法
		Compression   int8 `json:"compression"`   // 压缩方式
	}

	// Auth 认证消息
	Auth struct {
		AuthID     string            `json:"auth_id"`     // 认证ID, 如电话号码等
		AuthType   string            `json:"auth_type"`   // 认证类型, 如strong, weak
		VerifyType string            `json:"verify_type"` // 校验方式, 如Phone
		Verify     string            `json:"verify"`      // 校验令牌, 如验证码
		Info       map[string]string `json:"info"`        // 额外信息
	}

	// Config 配置消息
	Config struct {
		ChatConfig ChatConfig `json:"chat_config"`
		ASRConfig  ASRConfig  `json:"asr_config"`
		TTSConfig  TTSConfig  `json:"tts_config"`
	}

	// ChatConfig 对话配置
	ChatConfig struct {
	}

	// ASRConfig ASR配置
	ASRConfig struct {
		Format     string `json:"format"`      // 音频容器格式
		Codec      string `json:"codec"`       // 编码方式i
		Rate       int    `json:"rate"`        // 采样频率
		Bits       int    `json:"bits"`        // 比特率
		Channels   int    `json:"channels"`    // 声道数
		ResultType string `json:"result_type"` // 返回方式, full为全量, single为增量
	}

	// TTSConfig TTS配置
	TTSConfig struct {
		Format       string  `json:"format"`        // 音频容器格式
		Codec        string  `json:"codec"`         // 编码方式i
		Rate         int     `json:"rate"`          // 采样频率
		Bits         int     `json:"bits"`          // 比特率
		Channels     int     `json:"channels"`      // 声道数
		ResultType   string  `json:"result_type"`   // 返回方式, full为全量, single为增量
		SpeechRate   float32 `json:"speech_rate"`   // 语速, 服务端配置
		LoudnessRate float32 `json:"loudness_rate"` // 音量, 服务端配置
		PitchRate    float32 `json:"pitch_rate"`    // 音高, 服务端配置
		Lang         string  `json:"lang"`          // 语种, 服务端配置
	}

	// Cmd 命名消息
	Cmd struct {
		Command CType  `json:"command"` // 命令类型
		Content string `json:"content"` // 命令内容
	}

	// Resp 响应消息
	Resp struct{}

	// Err 错误消息
	Err struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
)

// MMarshal 序列化消息
func MMarshal(m *Message, compression, serialization int8) (data []byte, err error) {
	// 序列化
	switch serialization {
	case JSON:
		if data, err = json.Marshal(m); err != nil {
			return nil, err
		}
	default:
		return nil, unimplement
	}

	// 压缩
	switch compression {
	case GZIP:
		if data, err = util.GzipCompress(data); err != nil {
			return nil, err
		}
	default:
		return nil, unimplement
	}
	return data, nil
}

// MUnmarshal 反序列化消息
func MUnmarshal(data []byte, compression, serialization int8) (m *Message, err error) {
	// 解压
	switch compression {
	case GZIP:
		if data, err = util.GzipDecompress(data); err != nil {
			return nil, err
		}
	default:
		return nil, unimplement
	}

	// 反序列化
	m = &Message{}
	switch serialization {
	case JSON:
		err = json.Unmarshal(data, m)
	default:
		return nil, unimplement
	}
	return m, err
}

// DecodeMessage 从消息中解码 payload
func DecodeMessage(m *Message) (payload any, err error) {
	switch m.Type {
	case MAuth:
		return decodeMessage[Auth](m)
	case MConfig:
		return decodeMessage[Config](m)
	case MCmd:
		return decodeMessage[Cmd](m)
	case MResp:
		return decodeMessage[Resp](m)
	case MErr:
		return decodeMessage[Err](m)
	}
	return nil, unimplement
}

func decodeMessage[T any](m *Message) (*T, error) {
	var payload T
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func EncodeMessage(t MType, payload any) (m *Message, err error) {
	var data []byte

	// 序列化
	if data, err = json.Marshal(payload); err != nil {
		return nil, err
	}
	m = &Message{
		Type:      t,
		Payload:   data,
		Timestamp: time.Now(),
	}
	return m, nil
}

func EncodeMErr(code int, msg string) (*Message, error) {
	e := &Err{Code: code, Message: msg}
	return EncodeMessage(MErr, e)
}

var (
	DecodeMsgErr []byte // 解码消息错误
	UnSupportErr []byte // 不支持的消息类型
)

// init 初始化一些全局变量
func init() {
	var err error
	var m *Message
	// 解码消息错误
	if m, err = EncodeMErr(-1001, "decode message error"); err != nil {
		panic(fmt.Errorf("[protocol] DecodeMsgErr EncodeMErr error %s", err))
	}
	if DecodeMsgErr, err = MMarshal(m, GZIP, JSON); err != nil {
		panic(fmt.Errorf("[protocol] DecodeMsgErr MMarshal error %s", err))
	}

	// 不支持的消息类型错误
	if m, err = EncodeMErr(-1002, "un-support message type error"); err != nil {
		panic(fmt.Errorf("[protocol] UnSupportErr EncodeMErr error %s", err))
	}
	if UnSupportErr, err = MMarshal(m, GZIP, JSON); err != nil {
		panic(fmt.Errorf("[protocol] UnSupportErr Marshal error %s", err))
	}
}
