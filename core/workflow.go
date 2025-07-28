package core

import "github.com/xh-polaris/psych-pkg/app"

// WorkFlow 工作流, 编排对话中数据流转
type WorkFlow interface {
	// Orchestrate 编排工作流
	Orchestrate(*WorkFlowConfig) error
	// Run 开始执行工作流
	Run() (err error)
}

type WorkFlowConfig struct {
	ChatConfig   *app.ChatSetting
	ReportConfig *app.ReportSetting
	ASRConfig    *app.ASRSetting
	TTSConfig    *app.TTSSetting
}
