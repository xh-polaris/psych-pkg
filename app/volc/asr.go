// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package volc

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

// ASR协议常量
const (
	ProtocolVersion     = byte(0b0001)
	DefaultHeaderSize   = 0b0001
	FullClientRequest   = byte(0b0001)
	AudioOnlyRequest    = byte(0b0010)
	FullServerResponse  = byte(0b1001)
	ServerAck           = byte(0b1011)
	ServerErrorResponse = byte(0b1111)

	NoSequence      = byte(0b0000) // no check sequence
	PosSequence     = byte(0b0001)
	NegSequence     = byte(0b0010)
	NegWithSequence = byte(0b0011)
	NegSequence1    = byte(0b0011)

	NoSerialization = byte(0b0000)
	JSON            = byte(0b0001)

	NoCompression = byte(0b0000)
	GZIP          = byte(0b0001)

	parseNum = byte(0b00001111)
)

var (
	DefaultASRSetting = &app.ASRSetting{
		Format:     "pcm",
		Codec:      "raw",
		Rate:       16000,
		Bits:       16,
		Channels:   1,
		ModelName:  "bigapp",
		EnablePunc: true,
		EnableDdc:  false,
		ResultType: "single",
	}
	negHeader = getHeader(AudioOnlyRequest, NegSequence, JSON, GZIP, byte(0))
	posHeader = getHeader(AudioOnlyRequest, PosSequence, JSON, GZIP, byte(0))
)

// VcASRApp 是火山引擎的大模型语音识别
// 前后端一个长连接, 每轮对话, 收到first后建立新的asr链接
// 双向流式会增量返回, 流式则是最后一个包或15s后返回, 单包时长100~200ms最优
type VcASRApp struct {
	wsx *wsx.WSClient
	ini chan struct{} // 同步机制, 确保Receive在wsx初始化后

	// 鉴权与配置
	appId      string
	accessKey  string
	resourceId string
	url        string
	setting    *app.ASRSetting

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
		ini:        make(chan struct{}),
		appId:      setting.AppID,
		accessKey:  setting.AccessKey,
		resourceId: setting.ResourceId,
		url:        setting.Url,
		setting:    setting,
		seq:        1,
		uSession:   uSession,
		dSession:   util.NewUID(),
	}
	asr.buildHTTPHeader()
	return asr
}

// dial 建立ws链接
func (asr *VcASRApp) dial(ctx context.Context) (err error) {
	asr.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), asr.url, asr.header)
	asr.ini <- struct{}{} // 写入消息, 允许Receive开始read
	return err
}

// start 完成应用层协议握手
func (asr *VcASRApp) start() (err error) {
	var payload []byte
	setting := asr.setting
	// 协商配置参数
	req := map[string]any{
		// 用户参数
		"user": map[string]any{
			"uid": asr.uSession,
		},
		// 音频参数
		"audio": map[string]any{
			"format":      setting.Format,
			"sample_rate": setting.Rate,
			"bits":        setting.Bits,
			"channels":    setting.Channels,
			"codec":       setting.Codec,
		},
		"request": map[string]any{
			"app_name":    setting.ModelName,
			"enable_punc": setting.EnablePunc,
			"result_type": setting.ResultType,
		},
	}
	// 序列化为字节
	if payload, err = json.Marshal(req); err != nil {
		return err
	}
	// gzip压缩
	if payload, err = util.GzipCompress(payload); err != nil {
		return err
	}
	// 组装full client request, full client request = header + sequence + payload
	header := getHeader(FullClientRequest, PosSequence, JSON, GZIP, byte(0))
	seq := util.IntToBytes(asr.seq)
	size := util.IntToBytes(len(payload))
	fullClientRequest := util.BuildBytes(header, seq, size, payload)
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
		if err = asr.start(); err != nil {
			return
		}
	}

	var payload, header []byte
	ctx = util.NNCtx(ctx)

	// 判断是否最后一个包, 若是则负载为空
	header = negHeader
	if !app.IsLastASR(data) { // 不是负包则正常处理
		header = posHeader
		payload, err = util.GzipCompress(data)
		if err != nil {
			return err
		}
	}

	// 发送音频流
	asr.seq++
	seq := util.IntToBytes(asr.seq)
	payloadSize := util.IntToBytes(len(payload))
	audioOnlyRequest := util.BuildBytes(header, seq, payloadSize, payload)
	if err = asr.wsx.WriteBytes(audioOnlyRequest); err != nil {
		return err
	}
	return nil
}

// Receive 接受响应
func (asr *VcASRApp) Receive(ctx context.Context) (text string, err error) {
	var res []byte
	var mt int
	if asr.wsx == nil { // 避免初始化前监听wsx导致空指针错误
		<-asr.ini
		if asr.wsx == nil { // 出现这种情况多半是因为wsx还没建立就关闭了, 需要再检测一次避免空指针问题
			return "", nil
		}
	}
	if mt, res, err = asr.wsx.Read(); err == nil {
		switch mt {
		case websocket.BinaryMessage:
			return asr.receiveBytes(res)
		case websocket.TextMessage:
			return asr.receiveText(res)
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

// receiveBytes 接收到字节流
func (asr *VcASRApp) receiveBytes(res []byte) (text string, err error) {
	data, seq, err := parse(res)
	// seq 小于0 表示这是最后一个包, 后续没有了, 暂时没有通过这个来中止
	if err != nil || seq < 0 {
		return "", err
	}
	// 反序列化, 提取识别后的文字
	r := make(map[string]any)
	if err = json.Unmarshal(data, &r); err != nil {
		return "", err
	}
	if text, ok := r["result"].(map[string]any)["text"].(string); ok {
		return text, nil
	}
	return "", fmt.Errorf("[volc asr] receiveBytes: invalid result")
}

// Close 释放资源
func (asr *VcASRApp) Close() (err error) {
	if asr.wsx != nil {
		return asr.wsx.Close()
	}
	close(asr.ini)
	return
}

// parse 解析响应帧
func parse(res []byte) (data []byte, seq int, err error) {
	if res == nil || len(res) == 0 {
		return
	}
	msgType, msgCompression, payload := (res[1]>>4)&parseNum, res[2]&0x0f, res[12:]
	// sequence 4bytes
	if seq, err = util.BytesToInt(res[4:8]); err != nil {
		return nil, 0, err
	}
	switch msgType {
	case FullServerResponse:
		if msgCompression == GZIP {
			data, err = util.GzipDecompress(payload)
			return data, seq, err
		} else {
			return payload, seq, nil
		}
	case ServerAck:
		return payload, seq, nil
	case ServerErrorResponse:
		return payload, seq, fmt.Errorf("code: %d, msg: %s", seq, string(payload))
	}
	return nil, 0, nil
}

// buildHTTPHeader 构造鉴权请求头
func (asr *VcASRApp) buildHTTPHeader() {
	asr.header = http.Header{
		"X-Tt-Logid":        []string{asr.dSession},
		"X-Api-Resource-Id": []string{asr.resourceId},
		"X-Api-Access-Key":  []string{asr.accessKey},
		"X-Api-App-Key":     []string{asr.appId},
		"X-Api-Connect-Id":  []string{asr.dSession},
	}
}

// getHeader 生成协议头
func getHeader(msgType, msgTypeSpecificFlags, serialMethod, compressionType, reserverData byte) []byte {
	header := make([]byte, 4)
	header[0] = (ProtocolVersion << 4) | DefaultHeaderSize
	header[1] = (msgType << 4) | msgTypeSpecificFlags
	header[2] = (serialMethod << 4) | compressionType
	header[3] = reserverData
	return header
}
