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

var DefaultMTTSSetting = &app.MTTSSetting{
	Namespace: "BidirectionalTTS",
	Speaker:   "zh_female_xinlingjitang_moon_bigtts",
	AudioParams: struct {
		Format       string `json:"format"`
		Rate         int32  `json:"rate"`
		Bit          int32  `json:"bit"`
		SpeechRate   int32  `json:"speech_rate"`
		LoudnessRate int32  `json:"loudness_rate"`
		Lang         string `json:"lang"`
	}{
		Format:       "pcm",
		Rate:         24000,
		Bit:          16000,
		SpeechRate:   0,
		LoudnessRate: 0,
		Lang:         "zh",
	},
}

// VcMTTSApp 是火山引擎的大模型语音合成
// 默认双向流式, 一次对话只需要建立一个链接即可使用到最后.
// 由于火山的文档写的有点语焉不详, 示例代码又没有明确的说明, 所以有些代码看着很冗余也只能先留着
type VcMTTSApp struct {
	dialOnce  sync.Once
	startOnce sync.Once
	// ws 连接
	wsx        *wsx.WSClient
	appKey     string
	accessKey  string
	speaker    string
	resourceId string
	url        string
	setting    *app.MTTSSetting
	params     *TTSReqParams

	// uSession, 一次对话的ID
	uSession string
	// header 是请求头, 携带鉴权信息
	header http.Header
}

func NewVcTTSApp(uSession, appKey, accessKey, speaker, resourceId, url string, setting *app.MTTSSetting) *VcMTTSApp {
	tts := &VcMTTSApp{
		appKey:     appKey,
		accessKey:  accessKey,
		speaker:    speaker,
		url:        url,
		resourceId: resourceId,
		setting:    setting,
		uSession:   uSession,
	}
	tts.buildHTTPHeader()
	return tts
}

// Dial 建立ws连接
func (app *VcMTTSApp) Dial(ctx context.Context) (err error) {
	app.dialOnce.Do(func() {
		app.wsx, err = wsx.NewWSClientWithDial(util.NNCtx(ctx), app.url, app.header)
	})
	return err
}

// Start 应用层协议握手
func (app *VcMTTSApp) Start() (err error) {
	app.startOnce.Do(func() {
		if err = app.startConnection(); err != nil {
			return
		}
		setting := app.setting
		namespace := setting.Namespace
		app.params = &TTSReqParams{
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
		if err = app.startTTSSession(namespace, app.params); err != nil {
			return
		}
	})
	return
}

// Send 发送请求
func (app *VcMTTSApp) Send(ctx context.Context, text string) (err error) {
	return app.sendTTSMessage(text)
}

// Receive 接收请求
func (app *VcMTTSApp) Receive(ctx context.Context) ([]byte, error) {
	for {
		msg, err := app.receiveMessage()
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
func (app *VcMTTSApp) Close() (err error) {
	if app.wsx == nil {
		return nil
	}
	if err = app.finishSession(); err != nil {
		return err
	}
	if err = app.finishConnection(); err != nil {
		return err
	}
	return app.wsx.Close()
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
func (app *VcMTTSApp) buildHTTPHeader() {
	app.header = http.Header{
		"X-Tt-Logid":        []string{app.uSession},
		"X-Api-Resource-Id": []string{app.resourceId},
		"X-Api-Access-Key":  []string{app.accessKey},
		"X-Api-App-Key":     []string{app.appKey},
		"X-Api-Connect-Id":  []string{app.uSession},
	}
}

// startConnection 建立application级别的连接
func (app *VcMTTSApp) startConnection() (err error) {
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
	if err = app.wsx.WriteBytes(frame); err != nil {
		logx.Error("[volc mtts] send StartConnection request: %w", err)
		return err
	}

	// Read ConnectionStarted message.
	mt, frame, err := app.wsx.Read()
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
func (app *VcMTTSApp) startTTSSession(namespace string, params *TTSReqParams) (err error) {
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
	msg.SessionID = app.uSession
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal StartSession request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send StartSession request: %w", err)
	}

	// Read SessionStarted message.
	mt, frame, err := app.wsx.Read()
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
func (app *VcMTTSApp) sendTTSMessage(text string) error {
	req := TTSRequest{
		Event:     int32(EventTaskRequest),
		Namespace: app.setting.Namespace,
		ReqParams: app.params,
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
	msg.SessionID = app.uSession
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal TaskRequest request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send TaskRequest request: %w", err)
	}
	return nil
}

// receiveMessage 从ws中接受消息
func (app *VcMTTSApp) receiveMessage() (*Message, error) {
	mt, frame, err := app.wsx.Read()
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
func (app *VcMTTSApp) finishSession() error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("[volc mtts] create FinishSession request message: %w", err)
	}
	msg.Event = int32(EventFinishSession)
	msg.SessionID = app.uSession
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("[volc mtts] marshal FinishSession request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send FinishSession request: %w", err)
	}
	return nil
}

// finishConnection 关闭连接
func (app *VcMTTSApp) finishConnection() error {
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

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("[volc mtts] send FinishConnection request: %w", err)
	}

	mt, frame, err := app.wsx.Read()
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
