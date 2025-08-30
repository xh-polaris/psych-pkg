package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
	"net/http"
	"sync/atomic"
)

var _ app.TTSApp = (*VcMTTSApp)(nil)

func init() {
	app.TTSRegister("volc-model", NewVcMTTSApp)
}

// VcMTTSApp 是火山引擎的大模型语音合成
// 默认双向流式websocketV3, 一次对话只需要建立一个链接即可使用到最后.
// 每次转换用一个session, 一个链接可以复用多个session, 可以使用CancelSession结束当前session(未实现)
type VcMTTSApp struct {
	dialOnce  atomic.Bool
	startOnce atomic.Bool
	// ws 连接
	wsx *wsx.WSClient
	ini chan struct{}

	appId     string
	accessKey string
	url       string
	setting   *app.TTSSetting
	params    *TTSReqParams

	// uSession, 一次对话的ID
	uSession string

	// header 是请求头, 携带鉴权信息
	header http.Header
}

// NewVcMTTSApp 创建一个大模型TTS App
func NewVcMTTSApp(uSession string, setting *app.TTSSetting) app.TTSApp {
	tts := &VcMTTSApp{
		ini:       make(chan struct{}),
		appId:     setting.AppID,
		accessKey: setting.AccessKey,
		url:       setting.Url,
		setting:   setting,
		uSession:  uSession,
	}
	tts.buildHTTPHeader()
	return tts
}

// dial 建立ws连接
func (tts *VcMTTSApp) dial(ctx context.Context) (err error) {
	if tts.dialOnce.CompareAndSwap(false, true) {
		tts.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), tts.url, tts.header)
		tts.ini <- struct{}{}
		if err != nil {
			tts.dialOnce.CompareAndSwap(true, false) // 成功建立连接后则不再次建立连接
		}
	}
	return err
}

// start 建立Connection
func (tts *VcMTTSApp) start() (err error) {
	if tts.startOnce.CompareAndSwap(false, true) {
		if err = tts.startConnection(); err != nil { // 建立connection
			tts.startOnce.CompareAndSwap(true, false)
			return
		}
		setting := tts.setting // 配置tts参数
		tts.params = &TTSReqParams{
			Speaker: setting.Speaker,
			AudioParams: &AudioParams{
				Format:     setting.AudioParams.Format,
				SampleRate: setting.AudioParams.Rate,
				SpeechRate: setting.AudioParams.SpeechRate,
				BitRate:    setting.AudioParams.Bits,
				Volume:     setting.AudioParams.LoudnessRate,
				Lang:       setting.AudioParams.Lang,
			},
			Additions: "{\"disable_markdown_filter\": \"true\"}", // 过滤markdown
		}
	}
	return
}

// Send 发送请求
func (tts *VcMTTSApp) Send(ctx context.Context, text string) (err error) {
	if err = tts.dial(ctx); err != nil { // 建立ws连接
		return
	}
	if err = tts.start(); err != nil { // 建立connection
		return
	}
	if app.IsFirstTTS(text) { // 首包, 建立session
		return tts.startSession()
	} else if app.IsLastTTS(text) { // 尾包, 结束session
		return tts.finishSession()
	}
	return tts.sendTTSMessage(text)
}

// Receive 接收请求
func (tts *VcMTTSApp) Receive(ctx context.Context) ([]byte, error) {
	for {
		if tts.wsx == nil {
			<-tts.ini
			if tts.wsx == nil {
				return nil, nil
			}
		}
		msg, err := tts.receiveMessage()
		if err != nil {
			return nil, err
		}
		switch msg.MsgType {
		case MsgTypeFullServerResponse: // 收到服务器完整响应
			switch msg.EventType {
			case EventType_ConnectionStarted: // connection建立成功
				logx.Info("[volc mtts] Receive Connection success")
			case EventType_SessionStarted: // session 建立成功
				logx.Info("[volc mtts] Receive Session success")
			case EventType_SessionFinished: // session 结束
				logx.Info("[volc mtts] Receive Session Finish success")
			case EventType_ConnectionFinished: // connection结束
				logx.Info("[volc mtts] Receive Connection Finish success")
			}
		case MsgTypeAudioOnlyServer: // 接收到音频响应
			return msg.Payload, nil
		case MsgTypeError: // 接收到错误
			return nil, fmt.Errorf("[volc mtts] Receive Error: (code=%d): %s", msg.ErrorCode, msg.Payload)
		default:
			return nil, fmt.Errorf("[volc mtts] Received unexpected message type: %s", msg.MsgType)
		}
	}
}

// Close 关闭连接释放资源
func (tts *VcMTTSApp) Close() (err error) {
	if tts.wsx == nil {
		return nil
	}
	if err = tts.finishConnection(); err != nil {
		return err
	}
	return tts.wsx.Close()
}

// buildHTTPHeader 构造请求头
func (tts *VcMTTSApp) buildHTTPHeader() {
	tts.header = http.Header{
		"X-Api-Resource-Id": []string{tts.setting.ResourceId},
		"X-Api-Access-Key":  []string{tts.accessKey},
		"X-Api-App-Key":     []string{tts.appId},
		"X-Api-Connect-Id":  []string{tts.uSession},
	}
}

// startConnection 建立application级别的连接
func (tts *VcMTTSApp) startConnection() (err error) {
	var msg *Message
	var frame []byte
	if msg, err = NewMessage(MsgTypeFullClientRequest, MsgTypeFlagWithEvent); err != nil {
		return fmt.Errorf("[volc mtts] create StartSession request message: %w", err)
	}
	msg.EventType = EventType_StartConnection
	msg.Payload = []byte("{}")
	if frame, err = msg.Marshal(); err != nil {
		return fmt.Errorf("[volc mtts] marshal StartConnection request message: %w", err)
	}
	if err = tts.wsx.WriteBytes(frame); err != nil {
		logx.Error("[volc mtts] send StartConnection request: %w", err)
		return err
	}
	return
}

// startSession 开启TTSSession, 一个session对应一轮转换
func (tts *VcMTTSApp) startSession() (err error) {
	req := TTSRequest{
		Event:     int32(EventType_StartSession),
		Namespace: tts.setting.Namespace,
		ReqParams: tts.params,
	}
	payload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal StartSession request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create StartSession request message: %w", err)
	}
	msg.EventType = EventType_StartSession
	msg.SessionID = tts.uSession
	msg.Payload = payload

	frame, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal StartSession request message: %w", err)
	}

	if err = tts.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send StartSession request: %w", err)
	}
	return
}

// sendTTSMessage 发送一条tts消息
func (tts *VcMTTSApp) sendTTSMessage(text string) error {
	req := TTSRequest{
		Event:     int32(EventType_TaskRequest),
		Namespace: tts.setting.Namespace,
		ReqParams: tts.params,
	}
	req.ReqParams.Text = text
	payload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal TaskRequest request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create TaskRequest request message: %w", err)
	}
	msg.EventType = EventType_TaskRequest
	msg.SessionID = tts.uSession
	msg.Payload = payload

	frame, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal TaskRequest request message: %w", err)
	}

	if err = tts.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send TaskRequest request: %w", err)
	}
	return nil
}

// receiveMessage 从ws中接受消息
func (tts *VcMTTSApp) receiveMessage() (*Message, error) {
	mt, frame, err := tts.wsx.Read()
	if err != nil {
		return nil, err
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return nil, fmt.Errorf("[volc mtts] unexpected Websocket message type: %d", mt)
	}

	msg, err := NewMessageFromBytes(frame)
	if err != nil {
		return nil, fmt.Errorf("[volc mtts] unmarshal response message: %w", err)
	}
	return msg, nil
}

// finishSession 关闭session
func (tts *VcMTTSApp) finishSession() error {
	msg, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create FinishSession request message: %w", err)
	}
	msg.EventType = EventType_FinishSession
	msg.SessionID = tts.uSession
	msg.Payload = []byte("{}")

	frame, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal FinishSession request message: %w", err)
	}

	if err = tts.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send FinishSession request: %w", err)
	}
	return nil
}

// finishConnection 关闭连接
func (tts *VcMTTSApp) finishConnection() error {
	msg, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create FinishConnection request message: %w", err)
	}
	msg.EventType = EventType_FinishConnection
	msg.Payload = []byte("{}")

	frame, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal FinishConnection request message: %w", err)
	}

	if err = tts.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send FinishConnection request: %w", err)
	}
	return nil
}
