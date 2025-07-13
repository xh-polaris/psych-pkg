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
	// MTTSSetting tts设置
	MTTSSetting struct {
		AppID       string `json:"app_id"`
		Namespace   string `json:"namespace"`
		Speaker     string `json:"speaker"`
		ResourceId  string `json:"resourceId"`
		AudioParams struct {
			Format       string `json:"format"`        // 音频格式
			Rate         int32  `json:"rate"`          // 采用频率
			Bit          int32  `json:"bit"`           // 比特率
			SpeechRate   int32  `json:"speech_rate"`   // 语速 (volc)取值范围[-50,100]，100代表2.0倍速，-50代表0.5倍数
			LoudnessRate int32  `json:"loudness_rate"` // 音量 (volc)取值范围[-50,100]，100代表2.0倍音量，-50代表0.5倍音量
			Lang         string `json:"lang"`
		} `json:"audio_params"`
	}

	TTSSetting struct {
		App struct {
			AppID   string `json:"app_id"`  // AppID, 应用标识, 平台上查询
			Token   string `json:"token"`   // 默认值, access_token
			Cluster string `json:"cluster"` // 集群名称, 平台上查询
		} `json:"app"`
		User struct {
			Uid string `json:"uid"` // 用户ID, 这里就用uSession
		} `json:"user"`
		Audio struct {
			Language    string  `json:"language"`     // 语言
			VoiceType   string  `json:"voice_type"`   // 发言人
			Encoding    string  `json:"encoding"`     // 编码方式, 默认pcm
			Rate        int32   `json:"rate"`         // 比特率, 默认24000
			SpeedRatio  float32 `json:"speed_ratio"`  // 语速, 默认1.0
			VolumeRatio float32 `json:"volume_ratio"` // 音量, 默认1.0
			PitchRatio  float32 `json:"pitch_ratio"`  // 音准, 默认1.0
		} `json:"audio"`
		Request struct {
			ReqID     string `json:"req_id"`    // 请求id, 用dSession
			Text      string `json:"text"`      // 待识别文本
			TextType  string `json:"text_type"` // 文字类型,默认plain
			Operation string `json:"operation"` // 传输类型, 默认流式submit
		} `json:"request"`
	}
)

// ttsFactory TTSApp的构造函数类型
// 火山鉴权参数命名不统一, 这里做个说明:
// 在代码中appID是应用标识, appKey是应用token; appID应在setting中, appKey作为参数传入
// 在请求中appKey是应用标识, accessKey是应用token
type ttsFactory func(uSession, appKey, url string, setting any) TTSApp

// ttsProviders TTSApp的构造函数
var ttsProviders = make(map[string]ttsFactory)

// TTSRegister 注册一个TTSApp的构造函数
func TTSRegister(name string, factory ttsFactory) {
	ttsProviders[name] = factory
}

// NewTTSApp 构造TTSApp的工厂方法
func NewTTSApp(provider, uSession, appKey, url string, setting any) (TTSApp, error) {
	if factory, ok := ttsProviders[provider]; ok {
		return factory(uSession, appKey, url, setting), nil
	}
	return nil, NoFactory
}
