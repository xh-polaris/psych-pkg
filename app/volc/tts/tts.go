// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package tts

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
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
	defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}
)

// VcTTSApp 是火山引擎的常规文字转音频(非大模型)
// 每一次文本到音频的转换需要使用一个链接
// 这里Receive在每次连接更替时都会出现一个NormalErr然后被忽略, 感觉可能不是很优雅, 但结合使用场景来说能用, 一般都是获取到上次一次完整的响应后才会开始新的
type VcTTSApp struct {
	wsx *wsx.WSClient

	// 鉴权与配置
	appId     string
	accessKey string
	url       string
	setting   *volcTTSSetting

	// seq 发送的消息序列号
	seq int
	// uSession 是一次对话的记录
	uSession string
	// header 是请求头, 携带鉴权信息
	header http.Header
}

// NewVcTTSApp 构造一个新的
func NewVcTTSApp(uSession string, setting *app.TTSSetting) app.TTSApp {
	tts := &VcTTSApp{
		appId:     setting.AppID,
		accessKey: setting.AccessKey,
		url:       setting.Url,
		seq:       1,
		uSession:  uSession,
	}
	tts.buildHTTPHeader()
	tts.buildSetting(setting)
	return tts

}

// Dial 建立ws连接, 只有第一次调用建立链接, 后续调用不会建立, 以确保
func (tts *VcTTSApp) Dial(ctx context.Context) (err error) {
	tts.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), tts.url, tts.header)
	return err
}

// Send 发送待转换文字
func (tts *VcTTSApp) Send(ctx context.Context, text string) (err error) {
	if app.IsFirstTTS(text) { // 新一次, 重新建立链接
		return
	} else if app.IsLastTTS(text) {
		return
	}

	var input []byte
	tts.setting.Request.Text = text
	// 序列化输入
	if input, err = json.Marshal(tts.setting); err != nil {
		return err
	}
	// gzip压缩输入
	if input, err = util.GzipCompress(input); err != nil {
		return err
	}

	// 构建请求头, 依次是默认头, 有效长度, 有效负载
	payloadSize := util.I2BigEndBytes(len(input))
	clientRequest := util.BuildBytes(defaultHeader, payloadSize, input)
	if err = tts.wsx.WriteBytes(clientRequest); err != nil {
		return err
	}
	return nil
}

// Receive 获取转换后音频流
func (tts *VcTTSApp) Receive(ctx context.Context) (audio []byte, last bool, err error) {
	// 获取原始响应
	if audio, err = tts.wsx.ReadBytes(); err != nil {
		if wsx.IsNormal(err) { // normal异常说明本次连接结束, 开始了一轮新的, 因此直接返回
			return nil, true, nil
		}
		logx.Error("[volc tts] Receive: raw audio: ", string(audio))
		return nil, true, err
	}
	// 解析音频, 此处暂时没有考虑返回是否为最后一个包
	if audio, last, err = parseAudio(audio); err != nil {
		logx.Error("[volc tts] Receive: parse audio: ", err)
		return nil, true, err
	}
	return audio, last, nil
}

// Close 关闭连接释放资源
func (tts *VcTTSApp) Close() (err error) {
	if tts.wsx != nil {
		return tts.wsx.Close()
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
func (tts *VcTTSApp) buildHTTPHeader() {
	tts.header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer;%s", tts.accessKey)}}
}

func (tts *VcTTSApp) buildSetting(setting *app.TTSSetting) {
	set := &volcTTSSetting{}
	set.App.AppID, set.App.Token, set.App.Cluster = tts.appId, tts.accessKey, setting.ResourceId
	set.Audio.Language, set.Audio.VoiceType, set.Audio.Encoding = setting.AudioParams.Lang, setting.Speaker, setting.AudioParams.Format
	set.Audio.Rate, set.Audio.SpeedRate = setting.AudioParams.Bits, float32(100+setting.AudioParams.SpeechRate)/100
	set.Audio.VolumeRate, set.Audio.PitchRate = float32(100+setting.AudioParams.LoudnessRate)/100, 1.0
	if setting.AudioParams.ResultType == "single" {
		set.Request.Operation = optSubmit
	} else {
		set.Request.Operation = optQuery
	}
	tts.setting.User.Uid = tts.uSession
	tts.setting.Request.ReqID = tts.uSession
}

type volcTTSSetting struct {
	App struct {
		AppID   string `json:"app_id"`  // AppID, 应用标识, 平台上查询
		Token   string `json:"token"`   // 默认值, access_token
		Cluster string `json:"cluster"` // 集群名称, 平台上查询
	} `json:"app"`
	User struct {
		Uid string `json:"uid"` // 用户ID, 这里就用uSession
	} `json:"user"`
	Audio struct {
		Language   string  `json:"language"`     // 语言
		VoiceType  string  `json:"voice_type"`   // 发言人
		Encoding   string  `json:"encoding"`     // 编码方式, 默认pcm
		Rate       int32   `json:"rate"`         // 比特率, 默认24000
		SpeedRate  float32 `json:"speed_ratio"`  // 语速, 默认1.0
		VolumeRate float32 `json:"volume_ratio"` // 音量, 默认1.0
		PitchRate  float32 `json:"pitch_ratio"`  // 音准, 默认1.0
	} `json:"audio"`
	Request struct {
		ReqID     string `json:"req_id"`    // 请求id, 用dSession
		Text      string `json:"text"`      // 待识别文本
		TextType  string `json:"text_type"` // 文字类型,默认plain
		Operation string `json:"operation"` // 传输类型, 默认流式submit
	} `json:"request"`
}
