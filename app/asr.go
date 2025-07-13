// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import "context"

type (
	// ASRApp 是通用语音识别
	// 如果存在应用层面的握手过程需要由ASR内部实现
	ASRApp interface {
		// Dial 建立ws连接
		Dial(ctx context.Context) error
		// Send 发送音频流
		// 标识结束的音频流是一个全为1的字节
		Send(ctx context.Context, bytes []byte) error
		// Receive 接受文字响应 TODO: 暂时只有使用文字的需求, 后续若用到其余部分再迭代
		Receive(ctx context.Context) (string, error)
		// Close  关闭连接, 释放资源
		Close() error
	}
	ASRSetting struct {
		Format     string `json:"format"`      // 音频容器 (volc)pcm(pcm_s16le) / wav(pcm_s16le) / ogg
		Codec      string `json:"codec"`       // 编码方式 (volc)raw / opus，默认为 raw(pcm)
		Rate       int    `json:"rate"`        // 采样频率 (volc)默认为 16000，目前只支持16000
		Bits       int    `json:"bits"`        // 比特率  (volc)默认为 16。
		Channels   int    `json:"channels"`    // 声道个数 (volc)默认为 1
		ModelName  string `json:"model_name"`  // 模型名称 (volc)目前只有bigmodel
		EnablePunc bool   `json:"enable_punc"` // 启用标点
		EnableDdc  bool   `json:"enable_ddc"`  // 启用语义顺滑
		ResultType string `json:"result_type"` // 返回方式,full为全量, single为增量
	}
)

// asrFactory ASRApp的构造函数类型
type asrFactory func(uSession, appKey, accessKey, resourceId, url string, setting *ASRSetting) ASRApp

// asrProviders ASRApp的构造函数
var asrProviders = make(map[string]asrFactory)

// ASRRegister 注册一个ASRApp的构造函数
func ASRRegister(name string, factory asrFactory) {
	asrProviders[name] = factory
}

// NewASRApp 构造ASRApp的工厂方法
func NewASRApp(provider, uSession, appKey, accessKey, resourceId, url string, setting *ASRSetting) (ASRApp, error) {
	if factory, ok := asrProviders[provider]; ok {
		return factory(uSession, appKey, accessKey, resourceId, url, setting), nil
	}
	return nil, NoFactory
}
