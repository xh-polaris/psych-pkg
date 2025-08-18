// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package bailian

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/httpx"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var _ app.ChatApp = (*BLChatApp)(nil)

func init() {
	app.ChatRegister("bailian", NewBLChatApp)
}

// BLChatApp 是阿里云对话大模型应用
// 使用云端上下文管理，本地不管理聊天记录, 默认采用增量流式输出
type BLChatApp struct {
	appId     string
	accessKey string
	url       string

	header http.Header
	body   map[string]any
	// uSession (Upper Session) 是创建该App时由调用者设置的session
	uSession string
	// dSession (Down Session) 对应用百炼管理上下文时百炼生成的session
	// 如果不是百炼管理应该与uSession相同, 同时应该实现连接丢失时通过uSession获取到聊天记录(TODO 暂时没有实现)
	dSession string
}

// NewBLChatApp 创建一个百炼模型应用实例
func NewBLChatApp(uSession string, setting *app.ChatSetting) app.ChatApp {
	chat := &BLChatApp{
		appId:     setting.AppId,
		accessKey: setting.AccessKey,
		url:       setting.Url,
		header:    http.Header{},
		body:      make(map[string]any),
		uSession:  uSession,
	}

	// 初始化请求模板
	chat.body["input"] = make(map[string]string)
	// 设置增量流式响应
	chat.body["parameters"] = map[string]any{
		"incremental_output": true,
	}
	// 设置请求头,其中X-DashScope-SSE设置为enable，表示开启流式响应
	chat.header.Set("Authorization", "Bearer "+setting.AccessKey)
	chat.header.Set("Content-Type", "application/json")
	chat.header.Set("X-DashScope-SSE", "enable")
	return chat
}

// Call 非流式调用，暂时没用上
func (app *BLChatApp) Call(ctx context.Context, prompt, uSession string) (*app.ChatFrame, error) {
	// TODO implement
	panic("implement me")
}

// StreamCall 流式调用
func (app *BLChatApp) StreamCall(ctx context.Context, prompt, uSession string) (app.ChatAppScanner, error) {
	client, ctx := httpx.GetHttpClient(), util.NNCtx(ctx)
	// 设置调用提示词
	app.body["input"].(map[string]string)["prompt"] = prompt
	app.body["input"].(map[string]string)["session_id"] = app.dSession
	// 获取流式响应reader
	reader, err := client.StreamPost(app.url, app.header, app.body)
	if err != nil {
		return nil, err
	}
	return app.newBLChatAppScanner(reader), nil
}

// Close 释放相关资源
// BLChat暂时没有需要释放的资源
func (app *BLChatApp) Close() error {
	return nil
}

// newBLChatAppScanner 创建一个新的大模型对话结果对象
// 非流式的可以模拟成一次返回然后io.EOF
func (app *BLChatApp) newBLChatAppScanner(r io.ReadCloser) app.ChatAppScanner {
	return &BLChatAppScanner{
		app:        app,
		readCloser: r,
		scanner:    bufio.NewScanner(r),
	}
}

var _ = (app.ChatAppScanner)(nil)

// BLChatAppScanner 是百炼对话调用的流式响应
type BLChatAppScanner struct {
	id         uint // 对应cmd id
	app        *BLChatApp
	readCloser io.ReadCloser
	scanner    *bufio.Scanner
}

// bLRawChatData 是百炼模型的原始响应
type bLRawChatData struct {
	Output struct {
		SessionId    string `json:"session_id"`
		FinishReason string `json:"finish_reason"`
		Text         string `json:"text"`
	} `json:"output"`
	Usage struct {
	} `json:"usage"`
}

// Next 返回下一个读取到的对象或错误
func (s *BLChatAppScanner) Next() (*app.ChatFrame, error) {
	var data app.ChatFrame
	var err error

	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		// 跳过空行
		if line != "" {
			switch {
			// 解析id
			case strings.HasPrefix(line, "id:"):
				if data.Id, err = strconv.ParseUint(strings.TrimPrefix(line, "id:"), 10, 64); err != nil {
					logx.Error("[bailian chat] unmarshal", err)
					return nil, err
				}
			// 解析消息主体
			case strings.HasPrefix(line, "data:"):
				var raw bLRawChatData
				if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &raw); err != nil {
					logx.Error("[bailian chat] unmarshal", err)
					return nil, err
				}
				data.SessionId = raw.Output.SessionId
				// 记录百炼生成的sessionId
				if s.app.dSession == "" {
					s.app.dSession = raw.Output.SessionId
				}
				data.Content = raw.Output.Text
				data.Finish = raw.Output.FinishReason
				data.Timestamp = time.Now().Unix()
				return &data, nil
			}
		}
	}
	if err = s.scanner.Err(); err != nil {
		logx.Error("[bailian chat]", err)
		return nil, err
	}
	// 没有更多内容
	return nil, app.End
}

// GetID 获取id
func (s *BLChatAppScanner) GetID() uint {
	return s.id
}

// WithID 设置ID
func (s *BLChatAppScanner) WithID(id uint) app.ChatAppScanner {
	s.id = id
	return s
}

// Close 释放资源
func (s *BLChatAppScanner) Close() error {
	return s.readCloser.Close()
}
