package wsx

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/websocket"
	"github.com/xh-polaris/psych-pkg/errorx"
)

// wsx 用于在hertz框架中使用websocket协议

// wsHandler 是ws协议处理函数
type wsHandler func(ctx context.Context, conn *websocket.Conn)

// upgrader 默认配置的协议升级器, 用于将HTTP请求升级为WebSocket请求
// 没有跨域限制
var upgrader = websocket.HertzUpgrader{
	CheckOrigin: func(ctx *app.RequestContext) bool {
		return true
	},
}

// UpgradeWs 将Http协议升级为WebSocket协议
func UpgradeWs(ctx context.Context, c *app.RequestContext, handler wsHandler) error {
	// 尝试升级协议, 处理请求
	if err := upgrader.Upgrade(c, func(conn *websocket.Conn) { handler(ctx, conn) }); err != nil {
		return errorx.EndM(err, "upgrade ws failed")
	}
	return nil
}
