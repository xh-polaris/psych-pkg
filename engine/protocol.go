package engine

import "time"

// 定义前后端通信的协议

var (
	MMeta   MType = 0 // 协议元数据
	MAuth   MType = 1 // 认证消息
	MConfig MType = 2 // 配置消息
	MCmd    MType = 3 // 常规命令
	MResp   MType = 4 // 响应
)

var (
	CUserAudio CType = 1 // 用户音频输入
	CUserText  CType = 2 // 用户文字输入
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
		Version       int    `json:"version"`       // 协议版本
		Serialization string `json:"serialization"` // 序列化方法
		Compression   string `json:"compression"`   // 压缩方式
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
)
