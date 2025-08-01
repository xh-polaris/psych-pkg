// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import (
	"context"
)

type (
	// ChatApp 是对话大模型应用
	// 上下文管理应该由ChatApp来实现, 可能存在变量中也可能直接交由第三方平台管理
	// 调用方通过sessionId来标识这一轮对话记录
	ChatApp interface {
		// Call 非流式调用
		Call(ctx context.Context, prompt, sessionId string) (*ChatFrame, error)
		// StreamCall 流式调用, 默认应该采用增量输出, 即后续的输出不包括之前的输出
		StreamCall(ctx context.Context, msg string, sessionId string) (ChatAppScanner, error)
		// Close 关闭资源
		Close() error
	}

	ChatSetting struct {
		Id        string `json:"id"`
		Provider  string `json:"provider"`
		Url       string `json:"url"`
		AppId     string `json:"appId"`
		AccessKey string `json:"accessKey"`
	}

	// ChatAppScanner 用于获取流式输出
	ChatAppScanner interface {
		WithID(id uint) ChatAppScanner
		GetID() uint
		// Next 获取下一个输出
		Next() (*ChatFrame, error)
		Close() error
	}

	// ChatFrame 一次响应
	ChatFrame struct {
		// Id 消息编号, 每次响应都应该从头统计
		Id uint64 `json:"id"`
		// Content 响应内容
		Content string `json:"content"`
		// SessionId 上下文标识
		SessionId string `json:"session_id"`
		// Timestamp 秒级时间戳
		Timestamp int64 `json:"timestamp"`
		// Finish 是否完成, stop正常完成, interrupt打断
		Finish string `json:"finish"`
	}
)

// chatFactory ChatApp的构造函数类型
type chatFactory func(uSession string, setting *ChatSetting) ChatApp

// chatProviders ChatApp的构造函数
var chatProviders = make(map[string]chatFactory)

// ChatRegister 注册一个ChatApp的构造函数
func ChatRegister(name string, factory chatFactory) {
	chatProviders[name] = factory
}

// NewChatApp 构造ChatApp的工厂方法
func NewChatApp(uSession string, setting *ChatSetting) (ChatApp, error) {
	if factory, ok := chatProviders[setting.Provider]; ok {
		return factory(uSession, setting), nil
	}
	return nil, NoFactory
}
