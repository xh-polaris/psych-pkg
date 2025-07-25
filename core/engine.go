package core

// Engine 对话引擎
// 管理对话流程中的核心部分
type Engine interface {
	// Run 启动Engine
	Run()
	// Close 释放Engine资源
	Close() (err error)
}

type CloseChannel interface {
	Close()
}

type Channel[T any] struct {
	C     chan T
	close chan struct{}
}

func NewChannel[T any](size int, close chan struct{}) *Channel[T] {
	return &Channel[T]{
		C:     make(chan T, size),
		close: close,
	}
}

func (c *Channel[T]) Close() {
	close(c.C)
}

func (c *Channel[T]) Send(msg T) {
	select {
	case <-c.close:
	case c.C <- msg:
	}
}

type Action string

const (
	Read  Action = "read"
	Pong  Action = "pong"
	UMMsg        = "unmarshal message"
	DMsg         = "decode message"
)
