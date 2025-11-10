// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import "context"

var (
	FirstTTS = "start"
	LastTTS  = "end"
)

// IsFirstTTS 判断是否是开始包
func IsFirstTTS(data string) bool {
	return data == FirstTTS
}

// IsLastTTS 判断是否是结束包
func IsLastTTS(data string) bool {
	return data == LastTTS
}

type (
	// TTSApp 是语音合成大模型
	// 如果不支持长连接实现有间隔的多轮合成, 则需在TTS内部维护链接刷新
	// 如果存在应用层面的握手过程需要由TTS内部实现
	TTSApp interface {
		// Dial 建立连接(包括配置过程)
		Dial(ctx context.Context) error
		// Send 发送文字请求
		Send(ctx context.Context, texts string) error
		// Receive 接受音频流响应
		Receive(ctx context.Context) ([]byte, bool, error)
		// Close 断开连接, 释放资源
		Close() error
	}
	// TTSSetting tts设置
	TTSSetting struct {
		Id          string   `json:"id"`
		Provider    string   `json:"provider"`
		Url         string   `json:"url"`
		AppID       string   `json:"app_id"`
		AccessKey   string   `json:"access_key"`
		Namespace   string   `json:"namespace"`
		Speaker     string   `json:"speaker"`
		ResourceId  string   `json:"resourceId"` // 资源ID或ClusterID
		AudioParams struct { // 音频参数
			Format       string `json:"format"`        // 音频格式
			Codec        string `json:"codec"`         // 编码方式 (volc)raw / opus，默认为 raw(pcm)
			Rate         int32  `json:"rate"`          // 采用频率
			Bits         int32  `json:"bits"`          // 比特率
			Channels     int    `json:"channels"`      // 声道个数 (volc)默认为 1
			SpeechRate   int32  `json:"speech_rate"`   // 语速 (volc)取值范围[-50,100]，100代表2.0倍速，-50代表0.5倍数
			LoudnessRate int32  `json:"loudness_rate"` // 音量 (volc)取值范围[-50,100]，100代表2.0倍音量，-50代表0.5倍音量
			Lang         string `json:"lang"`          // 语言
			ResultType   string `json:"result_type"`   // 返回方式,full为全量, single为增量
		} `json:"audio_params"`
	}
)

// ttsFactory TTSApp的构造函数类型
// 火山鉴权参数命名不统一, 这里做个说明:
// 在代码中appID是应用标识, appKey是应用token; appID应在setting中, appKey作为参数传入
// 在请求中appKey是应用标识, accessKey是应用token
type ttsFactory func(uSession string, setting *TTSSetting) TTSApp

// ttsProviders TTSApp的构造函数
var ttsProviders = make(map[string]ttsFactory)

// TTSRegister 注册一个TTSApp的构造函数
func TTSRegister(name string, factory ttsFactory) {
	ttsProviders[name] = factory
}

// NewTTSApp 构造TTSApp的工厂方法
func NewTTSApp(uSession string, setting *TTSSetting) (TTSApp, error) {
	if factory, ok := ttsProviders[setting.Provider]; ok {
		return factory(uSession, setting), nil
	}
	return nil, NoFactory
}
