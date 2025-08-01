package core

// Pipe 是对工作流中每个数据处理节点的抽象, 每个管道都是一个或多个独立的协程, 管道间通过chan通信
// 为统一管理资源, 所有生产者由Close()方法关闭, 并close对应的chan, 消费者由ctx关闭, 且不负责关闭chan
// 对于即使生产者又是消费者的BiPipe
// InPipe  指的是消费输入但不生产输出的管道
// OutPipe 指的是不消费输入但生产输出的管道
// BiPipe  指的是既消费输入又生产输出的管道
type (
	// Pipe 管道的基本定义
	Pipe interface {
		Run()
		Close()
	}

	// InPipe 消费输入但不生产输出
	InPipe interface {
		Pipe
		In()
	}

	// OutPipe 不消费输入但生产输出
	OutPipe interface {
		Pipe
		Out()
	}

	// BiPipe 既消费输入又生产输出
	BiPipe interface {
		InPipe
		OutPipe
	}
)
