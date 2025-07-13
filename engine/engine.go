package engine

import (
	"context"
	"github.com/hertz-contrib/websocket"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
	"time"
)

// IEngine 对话引擎
// 管理对话流程中的核心部分
type IEngine interface {
	// Run 启动Engine
	Run() (err error)
	// Close 释放Engine资源
	Close() (err error)
}

type Engine struct {
	// ctx 上下文
	ctx context.Context

	// ws 与前端的websocket链接
	wsx  *wsx.HZWSClient
	meta Meta

	// AI Apps
	chat     app.ChatApp
	tts      app.TTSApp
	asr      app.ASRApp
	workflow WorkFlow

	// uSession 对话标识
	uSession string

	// 记录
	start time.Time         // 开始时间
	round int               // 对话轮数
	info  map[string]string // 基本信息
}

// NewEngine 创建一个新的对话引擎
func NewEngine(ctx context.Context, conn *websocket.Conn) *Engine {
	ctx = util.NNCtx(ctx)
	e := &Engine{
		ctx:      ctx,
		wsx:      wsx.NewHZWSClient(conn),
		uSession: util.NewUID(),
		start:    time.Now(),
		round:    0,
		info:     make(map[string]string),
	}
	return e
}

// Run 运行对话引擎, 获取前端输入并执行对应处理
func (e *Engine) Run() (err error) {
	var mt int        // 消息类型
	var data []byte   // 前端传入数据
	var action string // 行为
	var m *Message    // 消息

	for {
		// 从客户端读取信息
		mt, data, err = e.wsx.Read()
		action = "read"

		switch mt {
		case websocket.PingMessage: // Ping消息
			action = "write pong"
			err = e.wsx.Pong()

		case websocket.TextMessage: // 文本消息
			logx.Info("[engine] receive text message:", string(data)) // 正常情况下不应该收到文本消息

		case websocket.BinaryMessage: // 二进制消息
			action = "MUnmarshal"
			if m, err = MUnmarshal(data, e.meta.Compression, e.meta.Serialization); err == nil {
				if err = e.Handle(m); err != nil { // 处理消息
					return
				}
			}
		case websocket.CloseMessage: // 关闭消息
		}

		if err != nil {
			logx.CondError(!wsx.IsNormal(err), "[engine] %s error %s", action, err)
			return
		}
	}
}

// Handle 处理消息, 当err!=nil时意味着engine需要退出了
func (e *Engine) Handle(m *Message) (err error) {
	var payload any

	// 解码消息
	if payload, err = DecodeMessage(m); err != nil {
		logx.Error("[engine] DecodeMessage error: %s", err)
		// 解码失败要发送错误消息, 但不退出
		if err = e.wsx.WriteBytes(DecodeMsgErr); err != nil {
			logx.Error("[engine] WriteBytes error: %s", err)
			return err
		}
		return nil
	}

	switch m.Type {
	case MAuth: // 认证消息
		if auth, ok := payload.(Auth); ok {
			return e.auth(auth)
		}
	case MCmd: // 命令消息
		if cmd, ok := payload.(Cmd); ok {
			return e.cmd(cmd)
		}
	default: // 不支持的消息
		if err = e.wsx.WriteBytes(UnSupportErr); err != nil {
			logx.Error("[engine] WriteBytes error: %s", err)
			return err
		}
	}
	return nil
}

// auth 验证用户信息
func (e *Engine) auth(auth Auth) (err error) {
	// 获取前端验证输入
	// 校验
	// 返回校验信息
	return nil
}

// config 配置app
func (e *Engine) config() (err error) {
	// 获取
	// 配置
	// 响应
	return err
}

// cmd 处理命令消息
func (e *Engine) cmd(cmd Cmd) (err error) {
	return nil
}

// record 记录对话过程
func (e *Engine) record() (err error) {
	return err
}

// Close 释放engine的资源
func (e *Engine) Close() (err error) {
	// 释放资源
	return err
}
