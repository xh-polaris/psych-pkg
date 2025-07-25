package core

// WorkFlow 工作流, 编排对话中数据流转
type WorkFlow interface {
	// Run 开始执行工作流
	Run() (err error)
}
