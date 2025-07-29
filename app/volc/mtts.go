package volc

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
	"sync"
)

var _ app.TTSApp = (*VcMTTSApp)(nil)

func init() {
	app.TTSRegister("volc-model", NewVcMTTSApp)
}

// VcMTTSApp 是火山引擎的大模型语音合成
// 默认双向流式, 一次对话只需要建立一个链接即可使用到最后.
// 由于火山的文档写的有点语焉不详, 示例代码又没有明确的说明, 所以有些代码看着很冗余也只能先留着
type VcMTTSApp struct {
	dialOnce  sync.Once
	startOnce sync.Once
	// ws 连接
	wsx *wsx.WSClient

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
	tts.dialOnce.Do(func() {
		tts.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), tts.url, tts.header)
	})
	return err
}

// start 应用层协议握手
func (tts *VcMTTSApp) start() (err error) {
	tts.startOnce.Do(func() {
		if err = tts.startConnection(); err != nil {
			return
		}
		setting := tts.setting
		namespace := setting.Namespace
		tts.params = &TTSReqParams{
			Speaker: setting.Speaker,
			AudioParams: &AudioParams{
				Format:     setting.AudioParams.Format,
				SampleRate: setting.AudioParams.Rate,
				SpeechRate: setting.AudioParams.SpeechRate,
				BitRate:    setting.AudioParams.Bit,
				Volume:     setting.AudioParams.LoudnessRate,
				Lang:       setting.AudioParams.Lang,
			},
			Additions: map[string]string{
				"disable_markdown_filter": "true", // 过滤markdown
			},
		}
		if err = tts.startTTSSession(namespace, tts.params); err != nil {
			return
		}
	})
	return
}

// Send 发送请求
func (tts *VcMTTSApp) Send(ctx context.Context, text string) (err error) {
	if app.IsFirstTTS(text) || app.IsLastTTS(text) { // 这里不需要刷新链接, 所以跳过标识内容
		return
	}
	if err = tts.dial(ctx); err != nil {
		return
	}
	if err = tts.start(); err != nil {
		return
	}
	return tts.sendTTSMessage(text)
}

// Receive 接收请求
func (tts *VcMTTSApp) Receive(ctx context.Context) ([]byte, error) {
	for {
		msg, err := tts.receiveMessage()
		if err != nil {
			return nil, err
		}
		switch msg.Type {
		case MsgTypeFullServer: // 接收到文本响应
			logx.Info("[volc mtts] Receive text message (event=%s, session_id=%s): %s", Event(msg.Event), msg.SessionID, msg.Payload)
			if msg.Event == int32(EventSessionFinished) {
				return nil, nil
			}
			continue
		case MsgTypeAudioOnlyServer: // 接收到音频响应
			return msg.Payload, nil
		case MsgTypeError: // 接收到错误
			return nil, fmt.Errorf("[volc mtts] Receive Error: (code=%d): %s", msg.ErrorCode, msg.Payload)
		default:
			return nil, fmt.Errorf("[volc mtts] Received unexpected message type: %s", msg.Type)
		}
	}
}

// Close 关闭连接释放资源
func (tts *VcMTTSApp) Close() (err error) {
	if tts.wsx == nil {
		return nil
	}
	if err = tts.finishSession(); err != nil {
		return err
	}
	if err = tts.finishConnection(); err != nil {
		return err
	}
	return tts.wsx.Close()
}

// protocol 是火山tts的二进制帧协议
var protocol = NewBinaryProtocol()

func init() {
	// Initialize binary protocol settings.
	protocol.SetVersion(Version1)
	protocol.SetHeaderSize(HeaderSize4)
	protocol.SetSerialization(SerializationJSON)
	protocol.SetCompression(CompressionNone, nil)
	protocol.ContainsSequence = ContainsSequence
}

// buildHTTPHeader 构造请求头
func (tts *VcMTTSApp) buildHTTPHeader() {
	tts.header = http.Header{
		"X-Tt-Logid":        []string{tts.uSession},
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
	if msg, err = NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent); err != nil {
		return fmt.Errorf("[volc mtts] create StartSession request message: %w", err)
	}
	msg.Event = int32(EventStartConnection)
	msg.Payload = []byte("{}")
	if frame, err = protocol.Marshal(msg); err != nil {
		return fmt.Errorf("[volc mtts] marshal StartConnection request message: %w", err)
	}
	if err = tts.wsx.WriteBytes(frame); err != nil {
		logx.Error("[volc mtts] send StartConnection request: %w", err)
		return err
	}

	// Read ConnectionStarted message.
	mt, frame, err := tts.wsx.Read()
	if err != nil {
		logx.Error("[volc mtts] Read StartConnection request: %w", err)
		return err
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("[volc mtts] unexpected Websocket message type: %d", mt)
	}
	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		logx.Error("[volc mtts] unmarshal ConnectionStarted response message: %w", err)
		return err
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("[volc mtts]unexpected ConnectionStarted message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventConnectionStarted {
		return fmt.Errorf("[volc mtts] unexpected response event (%s) for StartConnection request", Event(msg.Event))
	}
	return nil
}

// startTTSSession 开启TTSSession, 应该是用于标识一段上下文
func (tts *VcMTTSApp) startTTSSession(namespace string, params *TTSReqParams) (err error) {
	req := TTSRequest{
		Event:     int32(EventStartSession),
		Namespace: namespace,
		ReqParams: params,
	}
	payload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal StartSession request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create StartSession request message: %w", err)
	}
	msg.Event = req.Event
	msg.SessionID = tts.uSession
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal StartSession request message: %w", err)
	}

	if err = tts.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send StartSession request: %w", err)
	}

	// Read SessionStarted message.
	mt, frame, err := tts.wsx.Read()
	if err != nil {
		return fmt.Errorf("[volc mtts] read SessionStarted response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("[volc mtts] unexpected Websocket message type: %d", mt)
	}

	// Validate SessionStarted message.
	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		return fmt.Errorf("[volc mtts] unmarshal SessionStarted response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("[volc mtts] unexpected SessionStarted message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventSessionStarted {
		return fmt.Errorf("[volc mtts] unexpected response event (%s) for StartSession request", Event(msg.Event))
	}
	return nil
}

// sendTTSMessage 发送一条tts消息
func (tts *VcMTTSApp) sendTTSMessage(text string) error {
	req := TTSRequest{
		Event:     int32(EventTaskRequest),
		Namespace: tts.setting.Namespace,
		ReqParams: tts.params,
	}
	req.ReqParams.Text = text
	payload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal TaskRequest request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create TaskRequest request message: %w", err)
	}
	msg.Event = req.Event
	msg.SessionID = tts.uSession
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
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

	msg, _, err := Unmarshal(frame, ContainsSequence)
	if err != nil {
		if len(frame) > 500 {
			frame = frame[:500]
		}
		return nil, fmt.Errorf("[volc mtts] unmarshal response message: %w", err)
	}
	return msg, nil
}

// finishSession 关闭session
func (tts *VcMTTSApp) finishSession() error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create FinishSession request message: %w", err)
	}
	msg.Event = int32(EventFinishSession)
	msg.SessionID = tts.uSession
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
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
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create FinishConnection request message: %w", err)
	}
	msg.Event = int32(EventFinishConnection)
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal FinishConnection request message: %w", err)
	}

	if err = tts.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send FinishConnection request: %w", err)
	}

	mt, frame, err := tts.wsx.Read()
	if err != nil {
		return fmt.Errorf("[volc mtts] read ConnectionFinished response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("[volc mtts] unexpected Websocket message type: %d", mt)
	}

	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		return fmt.Errorf("[volc mtts] unmarshal ConnectionFinished response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("[volc mtts] unexpected ConnectionFinished message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventConnectionFinished {
		return fmt.Errorf("[volc mtts] unexpected response event (%s) for FinishConnection request", Event(msg.Event))
	}
	return nil
}
