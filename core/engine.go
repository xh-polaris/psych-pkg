package core

import "sync"

// Engine 对话引擎
// 管理对话流程中的核心部分
type Engine interface {
	// Run 启动Engine
	Run()
	// Read 读取未解码消息
	Read() (mt int, data []byte, err error)
	// Write 写入编码后消息
	Write([]byte)
	// MWrite 写入未编码消息
	MWrite(MType, any)
	// Close 释放Engine资源
	Close() (err error)
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
