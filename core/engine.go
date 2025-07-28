package core

import "sync"

// Engine 对话引擎
// 管理对话流程中的核心部分
type Engine interface {
	// Run 启动Engine
	Run()
	// Close 释放Engine资源
	Close() (err error)
	// Session 获取会话标识
	Session() string
}

type CloseChannel interface {
	Close()
}

type Channel[T any] struct {
	once  sync.Once
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
	c.once.Do(func() { close(c.C) })
}

func (c *Channel[T]) Send(msg T) {
	select {
	case <-c.close:
		c.once.Do(func() { close(c.C) })
	case c.C <- msg:
		select {
		case <-c.close:
			c.once.Do(func() { close(c.C) })
		}
	}
}

type Action string

const (
	ARead   Action = "read"
	APong   Action = "pong"
	AUMMsg  Action = "unmarshal message"
	ADMsg   Action = "decode message"
	AConfig Action = "config"
	AAuth   Action = "auth"
)
