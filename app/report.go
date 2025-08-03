// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import "context"

type ( // ReportApp 是报告分析大模型应用
	ReportApp interface {
		// Call 获取报告结果
		Call(ctx context.Context, prompt string) (*Report, error)
		// Close 关闭资源
		Close() error
	}

	ReportSetting struct {
		Id        string `json:"id"`
		Provider  string `json:"provider"`
		Url       string `json:"url"`
		AppId     string `json:"appId"`
		AccessKey string `json:"accessKey"`
	}

	// Report 分析报表
	Report struct {
		Items []*ReportItem `json:"items"`
	}

	// ReportItem 报表分析结果单元
	ReportItem struct {
		// Group 字段分组, 同一个group的
		Group string `json:"group"`
		// Type 字段类型 string, number, array-string, array-number
		Type  string `json:"type"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
)

// reportFactory ReportApp的构造函数类型
type reportFactory func(uSession string, setting *ReportSetting) ReportApp

// reportProviders ReportApp的构造函数
var reportProviders = make(map[string]reportFactory)

// ReportRegister 注册一个ReportApp的构造函数
func ReportRegister(name string, factory reportFactory) {
	reportProviders[name] = factory
}

// NewReportApp 构造ReportApp的工厂方法
func NewReportApp(uSession string, setting *ReportSetting) (ReportApp, error) {
	if factory, ok := reportProviders[setting.Provider]; ok {
		return factory(uSession, setting), nil
	}
	return nil, NoFactory
}
