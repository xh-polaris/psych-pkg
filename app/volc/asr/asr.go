// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package asr

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
	"golang.org/x/net/context"
	"net/http"
)

var _ app.ASRApp = (*VcASRApp)(nil)

func init() {
	app.ASRRegister("volc", NewVcASRApp)
}

// VcASRApp 是火山引擎的大模型语音识别
// 前后端一个长连接, 每轮对话, 收到first后建立新的asr链接, 每次发送first前需要保证前一次的last已发送
// 双向流式会增量返回, 流式则是最后一个包或15s后返回, 单包时长100~200ms最优
type VcASRApp struct {
	wsx *wsx.WSClient
	ini chan struct{} // 同步机制, 确保Receive在wsx初始化后

	// 鉴权与配置
	setting *app.ASRSetting

	// seq 发送的消息序列号
	seq int
	// session
	uSession, dSession string
	// header 是请求头, 携带鉴权信息
	header http.Header
}

// NewVcASRApp 构造一个新的ASR App
func NewVcASRApp(uSession string, setting *app.ASRSetting) app.ASRApp {
	asr := &VcASRApp{
		ini:      make(chan struct{}),
		setting:  setting,
		seq:      1,
		uSession: uSession,
		dSession: util.NewUID(),
	}
	asr.buildHTTPHeader()
	return asr
}

// dial 建立ws链接
func (asr *VcASRApp) dial(ctx context.Context) (err error) {
	asr.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), asr.setting.Url, asr.header)
	asr.seq = 1           // 重置seq
	asr.ini <- struct{}{} // 写入消息, 允许Receive开始read
	return err
}

// start 完成应用层协议握手
func (asr *VcASRApp) start() (err error) {
	var payload []byte
	setting := asr.setting
	// 协商配置参数
	req := NewFullClientRequest(asr.uSession, setting.Format, setting.Codec, setting.Rate, setting.Bits,
		setting.Channels, setting.ModelName, true, setting.EnablePunc, setting.EnableDdc, false, false)
	// 序列化为字节
	if payload, err = json.Marshal(req); err != nil {
		return err
	}
	// gzip压缩
	if payload, err = util.GzipCompress(payload); err != nil {
		return err
	}
	// 组装full client request, full client request = header + sequence + payload
	seq := util.IntToBytes(asr.seq)
	size := util.IntToBytes(len(payload))
	fullClientRequest := util.BuildBytes(PosDefaultHeader, seq, size, payload)
	if err = asr.wsx.WriteBytes(fullClientRequest); err != nil {
		return err
	}
	return
}

// Send 发送音频流
func (asr *VcASRApp) Send(ctx context.Context, data []byte) (err error) {
	if app.IsFirstASR(data) { // first包, 建立新链接
		if err = asr.dial(ctx); err != nil {
			return
		}
		return asr.start()
	}

	var payload, header []byte
	ctx = util.NNCtx(ctx)

	header = AudioPosDefaultHeader
	if app.IsLastASR(data) { // 判断是否最后一个包, 若是则负载为空, 序号为负
		header = AudioNegDefaultHeader
		asr.seq = -asr.seq
	} else {
		payload, err = util.GzipCompress(data)
		if err != nil {
			return err
		}
	}

	// 发送音频流
	seq := util.IntToBytes(asr.seq)
	payloadSize := util.IntToBytes(len(payload))
	audioOnlyRequest := util.BuildBytes(header, seq, payloadSize, payload)
	if err = asr.wsx.WriteBytes(audioOnlyRequest); err != nil {
		return err
	}
	asr.seq++
	return nil
}

// Receive 接受响应
func (asr *VcASRApp) Receive(_ context.Context) (text string, err error) {
	var msg []byte
	var mt int
	if asr.wsx == nil { // 避免初始化前监听wsx导致空指针错误
		<-asr.ini
		if asr.wsx == nil { // 出现这种情况多半是因为wsx还没建立就关闭了, 需要再检测一次避免空指针问题
			return "", nil
		}
	}
	if mt, msg, err = asr.wsx.Read(); err == nil {
		switch mt {
		case websocket.BinaryMessage:
			resp := ParseResponse(msg)
			return resp.PayloadMsg.Result.Text, nil
		case websocket.TextMessage:
			return asr.receiveText(msg)
		default:
			return "", fmt.Errorf("[volc asr] Receive: invalid websocket message")
		}
	}
	return "", err
}

// receiveText 接受到文本消息, 暂无实际用途
func (asr *VcASRApp) receiveText(res []byte) (string, error) {
	logx.Info("[volc asr] receiveText: ", string(res))
	return "", nil
}

// Close 释放资源
func (asr *VcASRApp) Close() (err error) {
	if asr.wsx != nil {
		return asr.wsx.Close()
	}
	close(asr.ini)
	return
}

// buildHTTPHeader 构造鉴权请求头
func (asr *VcASRApp) buildHTTPHeader() {
	asr.header = http.Header{
		"X-Tt-Logid":        []string{asr.dSession},
		"X-Api-Resource-Id": []string{asr.setting.ResourceId},
		"X-Api-Access-Key":  []string{asr.setting.AccessKey},
		"X-Api-App-Key":     []string{asr.setting.AppID},
		"X-Api-Connect-Id":  []string{asr.dSession},
	}
}
