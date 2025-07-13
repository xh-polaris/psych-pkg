package engine

import "errors"

var WorkFlowEnd = errors.New("workflow is end")

// IWorkFlow 工作流
// 编排对话中数据流转
type IWorkFlow interface {
	// Run 开始执行工作流
	Run() (err error)
}

// WorkFlow 工作流编排
type WorkFlow struct {
	// 引擎的指针, 因为会用到一部分的字段
	*Engine
}

func (wf *WorkFlow) Run() (err error) {
	return
}
