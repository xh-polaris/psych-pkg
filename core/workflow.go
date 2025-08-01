package core

import (
	"context"
	"github.com/xh-polaris/psych-pkg/app"
)

// WorkFlow 工作流, 编排对话中数据流转
type WorkFlow interface {
	// Orchestrate 编排工作流
	Orchestrate(*WorkFlowConfig) error
	// Close 关闭工作流
	Close() error
	WithIn(in *Channel[*Cmd]) WorkFlow
	WithContext(ctx context.Context) WorkFlow
	WithClose(close chan struct{}) WorkFlow
}

type WorkFlowConfig struct {
	ChatConfig   *app.ChatSetting
	ReportConfig *app.ReportSetting
	ASRConfig    *app.ASRSetting
	TTSConfig    *app.TTSSetting
}
