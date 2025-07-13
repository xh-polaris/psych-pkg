// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package volc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
	"net/http"
)

var _ app.TTSApp = (*VcTTSApp)(nil)

func init() {
	app.TTSRegister("volc", NewVcTTSApp)
}

// TTS协议常量
const (
	optQuery  string = "query"  // 非流式
	optSubmit string = "submit" // 流式
)

// version: b0001 (4 bits)  header size: b0001 (4 bits) message type: b0001 (Full client request) (4bits)
// message type specific flags: b0000 (none) (4bits) message serialization method: b0001 (JSON) (4 bits)
// message compression: b0001 (gzip) (4bits) reserved data: 0x00 (1 byte)
var (
	defaultHeader     = []byte{0x11, 0x10, 0x11, 0x00}
	DefaultTTSSetting = &app.TTSSetting{
		App: struct {
			AppID   string `json:"app_id"`
			Token   string `json:"token"`
			Cluster string `json:"cluster"`
		}{
			AppID:   "",
			Token:   "access_token",
			Cluster: "",
		},
		User: struct {
			Uid string `json:"uid"`
		}{
			Uid: "",
		},
		Audio: struct {
			Language   string  `json:"language"`
			VoiceType  string  `json:"voice_type"`
			Encoding   string  `json:"encoding"`
			Rate       int32   `json:"rate"`
			SpeedRate  float32 `json:"speed_ratio"`
			VolumeRate float32 `json:"volume_ratio"`
			PitchRate  float32 `json:"pitch_ratio"`
		}{
			Language:   "",
			VoiceType:  "",
			Encoding:   "pcm",
			Rate:       24000,
			SpeedRate:  1.0,
			VolumeRate: 1.0,
			PitchRate:  1.0,
		},
		Request: struct {
			ReqID     string `json:"req_id"`
			Text      string `json:"text"`
			TextType  string `json:"text_type"`
			Operation string `json:"operation"`
		}{
			ReqID:     "",
			Text:      "",
			TextType:  "plain",
			Operation: optSubmit,
		},
	}
)

// VcTTSApp 是火山引擎的常规文字转音频(非大模型)
// 每一次文本到音频的转换需要使用一个链接, 但这个过程对于前端来说是被封装了的.
type VcTTSApp struct {
	wsx *wsx.WSClient

	// 鉴权与配置
	appKey  string
	url     string
	setting *app.TTSSetting

	// seq 发送的消息序列号
	seq int
	// uSession 是一次对话的记录, 由上层传入, dSession是一轮转换的记录, 由自己管理
	uSession, dSession string
	// header 是请求头, 携带鉴权信息
	header http.Header
}

// NewVcTTSApp 构造一个新的
func NewVcTTSApp(uSession, appKey, url string, setting any) app.TTSApp {
	if reSetting, ok := setting.(*app.TTSSetting); ok {
		tts := &VcTTSApp{
			appKey:   appKey,
			url:      url,
			setting:  reSetting,
			seq:      1,
			uSession: uSession,
			dSession: unStart,
		}
		tts.buildHTTPHeader()
		return tts
	}
	return nil
}

// Dial 建立ws连接, 只有第一次调用建立链接, 后续调用不会建立, 以确保
func (app *VcTTSApp) Dial(ctx context.Context) (err error) {
	app.dSession = util.NewUID()
	app.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), app.url, app.header)
	return err
}

// Start 完成应用层协议握手
func (app *VcTTSApp) Start() (err error) {
	// 大部分参数配置均已在外部完成, 这里是为了和其他的TTSApp操作对齐以及设置请求标识ID
	app.setting.User.Uid = app.dSession
	app.setting.Request.ReqID = app.dSession
	return nil
}

// Send 发送待转换文字
func (app *VcTTSApp) Send(ctx context.Context, text string) (err error) {
	var input []byte

	app.setting.Request.Text = text
	// 序列化输入
	if input, err = json.Marshal(app.setting); err != nil {
		return err
	}
	// gzip压缩输入
	if input, err = util.GzipCompress(input); err != nil {
		return err
	}

	// 构建请求头, 依次是默认头, 有效长度, 有效负载
	payloadSize := util.IntToBytes(len(input))
	clientRequest := util.BuildBytes(defaultHeader, payloadSize, input)
	if err = app.wsx.WriteBytes(clientRequest); err != nil {
		return err
	}
	return nil
}

// Receive 获取转换后音频流
func (app *VcTTSApp) Receive(ctx context.Context) (audio []byte, err error) {
	// 获取原始响应
	if audio, err = app.wsx.ReadBytes(); err != nil {
		logx.Error("[volc tts] Receive: raw audio: ", string(audio))
		return nil, err
	}
	// 解析音频, 此处暂时没有考虑返回是否为最后一个包
	if audio, _, err = parseAudio(audio); err != nil {
		logx.Error("[volc tts] Receive: parse audio: ", err)
		return nil, err
	}
	return audio, nil
}

// Close 关闭连接释放资源
func (app *VcTTSApp) Close() (err error) {
	if app.wsx != nil {
		return app.wsx.Close()
	}
	return
}

// parseAudio 解析音频响应
func parseAudio(res []byte) (audio []byte, isLast bool, err error) {
	headSize := res[0] & 0x0f
	messageType := res[1] >> 4
	messageTypeSpecificFlags := res[1] & 0x0f
	messageCompression := res[2] & 0x0f
	payload := res[headSize*4:]

	switch messageType {
	case 0xb:
		// 无有效响应
		if messageTypeSpecificFlags == 0 {
		} else {
			sequenceNumber := int32(binary.BigEndian.Uint32(payload[0:4]))
			payload = payload[8:]
			audio = append(audio, payload...)
			if sequenceNumber < 0 {
				isLast = true
			}
		}
	case 0xc: // 错误类型
		errMsg := payload[8:]
		if messageCompression == 1 {
			if errMsg, err = util.GzipDecompress(errMsg); err != nil {
				return
			}
		}
		err = errors.New(string(errMsg))
	case 0xf:
		payload = payload[4:]
		if messageCompression == 1 {
			payload, _ = util.GzipDecompress(payload)
		}
	}
	return
}

// buildHTTPHeader 构造鉴权请求头
func (app *VcTTSApp) buildHTTPHeader() {
	app.header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer;%s", app.appKey)}}
}
