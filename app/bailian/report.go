// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package bailian

import (
	"encoding/json"
	"fmt"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/httpx"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"net/http"
	"strings"
)

var _ app.ReportApp = (*BLReportApp)(nil)

// BLReportApp 是阿里云报告分析大模型应用
// 单次对话, 无需管理上下文
type BLReportApp struct {
	appId  string
	apiKey string
	url    string
	header http.Header
	body   map[string]any
}

// NewBLReportApp 创建一个百炼报告分析模型应用实例
func NewBLReportApp(appId string, apiKey string) app.ReportApp {
	report := &BLReportApp{
		appId:  appId,
		apiKey: apiKey,
		url:    fmt.Sprintf(baseUrl, appId),
		header: http.Header{},
		body:   make(map[string]any),
	}
	// 初始化请求模板
	report.body["input"] = make(map[string]string)
	report.body["parameters"] = map[string]any{}
	// 设置请求头,其中X-DashScope-SSE设置为enable，表示开启流式响应
	report.header.Set("Authorization", "Bearer "+apiKey)
	report.header.Set("Content-Type", "application/json")
	return report
}

// Call 调用模型获取分析报告
func (r *BLReportApp) Call(prompt string) (*app.Report, error) {
	var err error
	var report app.Report
	client := httpx.GetHttpClient()
	// 设置调用提示词
	r.body["input"].(map[string]string)["prompt"] = prompt
	res, err := client.Post(r.url, r.header, r.body)
	if err != nil {
		logx.Error("[bailian report]", err)
		return nil, err
	}
	text, ok := res["output"].(map[string]any)["text"].(string)
	if !ok {
		return nil, nil
	}
	// 去除markdown的`
	text = strings.Replace(text, "`", "", -1)
	err = json.Unmarshal([]byte(text), &report)
	return &report, err
}

// Close 释放相关资源
// BLChat暂时没有需要释放的资源
func (r *BLReportApp) Close() error {
	return nil
}
