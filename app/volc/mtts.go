package volc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
	"net/http"
)

var _ app.TTSApp = (*VcMTTSApp)(nil)

var DefaultTTSSetting = &app.TTSSetting{
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
// 默认双向流式, 一次对话使用一个链接
type VcMTTSApp struct {
	// ws 连接
	wsx        *wsx.WSClient
	appKey     string
	accessKey  string
	speaker    string
	resourceId string
	url        string
	setting    *app.TTSSetting
	params     *TTSReqParams

	// connId 连接id, 标识一次连接
	connId string
	// logId 服务端返回的logId, 用于定位问题
	logId string
	// uSession
	uSession string
	// header 是请求头, 携带鉴权信息
	header http.Header
}

func NewVcTTSApp(uSession, appKey, accessKey, speaker, resourceId, url string, setting *app.TTSSetting) *VcMTTSApp {
	connId := uuid.New().String()
	logId := genLogID()
	tts := &VcMTTSApp{
		appKey:     appKey,
		accessKey:  accessKey,
		speaker:    speaker,
		url:        url,
		resourceId: resourceId,
		setting:    setting,
		connId:     connId,
		logId:      logId,
		uSession:   uSession,
	}
	tts.buildHTTPHeader()
	return tts
}

// Dial 建立ws链接
func (app *VcMTTSApp) Dial() error {
	return app.DialCtx(nil)
}

// DialCtx 建立ws连接
func (app *VcMTTSApp) DialCtx(ctx context.Context) (err error) {
	app.wsx, err = wsx.NewWSClientWithDial(ctx, app.url, app.header)
	return err
}

// Start 应用层协议握手
func (app *VcMTTSApp) Start() (err error) {
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
	return
}

// startConnection 建立application级别的连接
func (app *VcMTTSApp) startConnection() (err error) {
	var msg *Message
	var frame []byte
	if msg, err = NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent); err != nil {
		return fmt.Errorf("create StartSession request message: %w", err)
	}
	msg.Event = int32(EventStartConnection)
	msg.Payload = []byte("{}")

	if frame, err = protocol.Marshal(msg); err != nil {
		return fmt.Errorf("marshal StartConnection request message: %w", err)
	}
	if err = app.wsx.WriteBytes(frame); err != nil {
		logx.Error("send StartConnection request: %w", err)
		return err
	}

	// Read ConnectionStarted message.
	mt, frame, err := app.wsx.Read()
	if err != nil {
		logx.Error("Read StartConnection request: %w", err)
		return err
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}
	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		glog.Infof("StartConnection response: %s", frame)
		logx.Error("unmarshal ConnectionStarted response message: %w", err)
		return err
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected ConnectionStarted message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventConnectionStarted {
		return fmt.Errorf("unexpected response event (%s) for StartConnection request", Event(msg.Event))
	}
	glog.Infof("Connection started (event=%s) connectID: %s, payload: %s", Event(msg.Event), msg.ConnectID, msg.Payload)
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
		return fmt.Errorf("marshal StartSession request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create StartSession request message: %w", err)
	}
	msg.Event = req.Event
	msg.SessionID = app.uSession
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal StartSession request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send StartSession request: %w", err)
	}

	// Read SessionStarted message.
	mt, frame, err := app.wsx.Read()
	if err != nil {
		return fmt.Errorf("read SessionStarted response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	// Validate SessionStarted message.
	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		glog.Infof("StartSession response: %s", frame)
		return fmt.Errorf("unmarshal SessionStarted response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected SessionStarted message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventSessionStarted {
		return fmt.Errorf("unexpected response event (%s) for StartSession request", Event(msg.Event))
	}
	return nil
}

// Send 发送请求
func (app *VcMTTSApp) Send(text string) (err error) {
	return app.sendTTSMessage(text)
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
		return fmt.Errorf("marshal TaskRequest request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create TaskRequest request message: %w", err)
	}
	msg.Event = req.Event
	msg.SessionID = app.uSession
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal TaskRequest request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send TaskRequest request: %w", err)
	}
	glog.Info("TaskRequest request is sent.")
	return nil
}

// Receive 接收请求
func (app *VcMTTSApp) Receive() []byte {
	for {
		msg, err := app.receiveMessage()
		if err != nil {
			glog.Errorf("Receive message error: %v", err)
			return nil
		}
		switch msg.Type {
		case MsgTypeFullServer:
			glog.Infof("Receive text message (event=%s, session_id=%s): %s", Event(msg.Event), msg.SessionID, msg.Payload)
			if msg.Event == int32(EventSessionFinished) {
				logx.Info("event type:", msg.Event)
				return nil
			}
			continue

		case MsgTypeAudioOnlyServer:
			glog.Infof("Receive audio message (event=%s): session_id=%s", Event(msg.Event), msg.SessionID)
			return msg.Payload

		case MsgTypeError:
			glog.Errorf("Receive Error message (code=%d): %s", msg.ErrorCode, msg.Payload)
			return nil
		default:
			glog.Errorf("Received unexpected message type: %s", msg.Type)
			return nil
		}
	}
}

// receiveMessage 从ws中接受消息
func (app *VcMTTSApp) receiveMessage() (*Message, error) {
	mt, frame, err := app.wsx.Read()
	if err != nil {
		return nil, err
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return nil, fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	msg, _, err := Unmarshal(frame, ContainsSequence)
	if err != nil {
		if len(frame) > 500 {
			frame = frame[:500]
		}
		glog.Infof("Data response: %s", frame)
		return nil, fmt.Errorf("unmarshal response message: %w", err)
	}
	return msg, nil
}

// Close 关闭连接释放资源
func (app *VcMTTSApp) Close() (err error) {
	if app.wsx == nil {
		return nil
	}
	if err = app.finishSession(); err != nil {
		glog.Errorf("Close session finished with error: %v", err)
	}
	if err = app.finishConnection(); err != nil {
		glog.Errorf("Close connection finished with error: %v", err)
	}
	return app.wsx.Close()
}

// finishSession 关闭session
func (app *VcMTTSApp) finishSession() error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create FinishSession request message: %w", err)
	}
	msg.Event = int32(EventFinishSession)
	msg.SessionID = app.uSession
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal FinishSession request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send FinishSession request: %w", err)
	}
	return nil
}

// finishConnection 关闭连接
func (app *VcMTTSApp) finishConnection() error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create FinishConnection request message: %w", err)
	}
	msg.Event = int32(EventFinishConnection)
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal FinishConnection request message: %w", err)
	}

	if err = app.wsx.WriteBytes(frame); err != nil {
		return fmt.Errorf("send FinishConnection request: %w", err)
	}

	// Read ConnectionStarted message.
	mt, frame, err := app.wsx.Read()
	if err != nil {
		return fmt.Errorf("read ConnectionFinished response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		glog.Infof("FinishConnection response: %s", frame)
		return fmt.Errorf("unmarshal ConnectionFinished response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected ConnectionFinished message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventConnectionFinished {
		return fmt.Errorf("unexpected response event (%s) for FinishConnection request", Event(msg.Event))
	}
	return nil
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
		"X-Tt-Logid":        []string{app.logId},
		"X-Api-Resource-Id": []string{app.resourceId},
		"X-Api-Access-Key":  []string{app.accessKey},
		"X-Api-App-Key":     []string{app.appKey},
		"X-Api-Connect-Id":  []string{app.connId},
	}
}
