// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import "context"

type (
	// TTSApp 是语音合成大模型
	// 如果存在应用层面的握手过程需要由TTS内部实现
	TTSApp interface {
		// Dial 建立ws连接
		Dial(ctx context.Context) error
		// Send 发送文字请求
		Send(ctx context.Context, texts string) error
		// Receive 接受音频流响应
		Receive(ctx context.Context) ([]byte, error)
		// Close 断开连接, 释放资源
		Close() error
	}
	// TTSSetting tts设置
	TTSSetting struct {
		Namespace   string   `json:"namespace"`
		Speaker     string   `json:"speaker"`
		ResourceId  string   `json:"resourceId"` // 资源ID或ClusterID
		Stream      bool     `json:"stream"`     // 是否流式
		AudioParams struct { // 音频参数
			Format       string `json:"format"`        // 音频格式
			Rate         int32  `json:"rate"`          // 采用频率
			Bit          int32  `json:"bit"`           // 比特率
			SpeechRate   int32  `json:"speech_rate"`   // 语速 (volc)取值范围[-50,100]，100代表2.0倍速，-50代表0.5倍数
			LoudnessRate int32  `json:"loudness_rate"` // 音量 (volc)取值范围[-50,100]，100代表2.0倍音量，-50代表0.5倍音量
			Lang         string `json:"lang"`
		} `json:"audio_params"`
	}
)

// ttsFactory TTSApp的构造函数类型
// 火山鉴权参数命名不统一, 这里做个说明:
// 在代码中appID是应用标识, appKey是应用token; appID应在setting中, appKey作为参数传入
// 在请求中appKey是应用标识, accessKey是应用token
type ttsFactory func(uSession, appId, accessKey, url string, setting *TTSSetting) TTSApp

// ttsProviders TTSApp的构造函数
var ttsProviders = make(map[string]ttsFactory)

// TTSRegister 注册一个TTSApp的构造函数
func TTSRegister(name string, factory ttsFactory) {
	ttsProviders[name] = factory
}

// NewTTSApp 构造TTSApp的工厂方法
func NewTTSApp(provider, uSession, appId, accessKey, url string, setting *TTSSetting) (TTSApp, error) {
	if factory, ok := ttsProviders[provider]; ok {
		return factory(uSession, appId, accessKey, url, setting), nil
	}
	return nil, NoFactory
}
