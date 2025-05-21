// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import "errors"

// End 用于流式中标识输出完成
var End = errors.New("[app] no more")

type (
	// ChatApp 是对话大模型应用
	// 上下文管理应该由ChatApp来实现, 可能存在变量中也可能直接交由第三方平台管理
	// 调用方通过sessionId来标识这一轮对话记录
	ChatApp interface {
		// Call 非流式调用
		Call(prompt, sessionId string) (*ChatFrame, error)
		// StreamCall 流式调用, 默认应该采用增量输出, 即后续的输出不包括之前的输出
		StreamCall(msg string, sessionId string) (ChatAppScanner, error)
		// Close 关闭资源
		Close() error
	}

	// ChatAppScanner 用于获取流式输出
	ChatAppScanner interface {
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

	// TTSApp 是语音合成大模型
	// 如果存在应用层面的握手过程需要由TTS内部实现
	TTSApp interface {
		// Dial 建立ws连接
		Dial() error
		// Send 发送文字请求
		Send(texts string) error
		// Receive 接受音频流响应
		Receive() []byte
		// Close 断开连接, 释放资源
		Close() error
	}

	// ASRApp 是通用语音识别
	// 如果存在应用层面的握手过程需要由ASR内部实现
	ASRApp interface {
		// Dial 建立ws连接
		Dial() error
		// Send 发送音频流
		// 标识结束的音频流是一个全为1的字节
		Send(bytes []byte) error
		// Receive 接受文字响应 TODO: 暂时只有使用文字的需求, 后续若用到其余部分再迭代
		Receive() (string, error)
		// Close  关闭连接, 释放资源
		Close() error
	}

	// ReportApp 是报告分析大模型应用
	ReportApp interface {
		// Call 获取报告结果
		Call(prompt string) (*Report, error)
		// Close 关闭资源
		Close() error
	}

	// Report 分析报表
	Report struct {
		Items ReportItem `json:"items"`
	}

	// ReportItem 报表分析结果单元
	ReportItem struct {
		// Group 字段分组, 同一个group的
		Group string `json:"group"`
		// Type 字段类型 string, number, array-string, array-number
		Type  string `json:"type"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
)
