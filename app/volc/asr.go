package volc

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/xh-polaris/psych-pkg/app"
	"github.com/xh-polaris/psych-pkg/util"
	"github.com/xh-polaris/psych-pkg/util/logx"
	"github.com/xh-polaris/psych-pkg/wsx"
	"golang.org/x/net/context"
	"net/http"
)

var _ app.ASRApp = (*VcASRApp)(nil)

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
	// 标识最后一个音频包
	lastOne           byte = 255
	DefaultASRSetting      = &app.ASRSetting{
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
// 一次识别用一个连接, 前端和后端建立一个长连接, 使用ASR时再建立和ASR的短连接, 使用后关闭
// 双向流式会增量返回, 流式则是最后一个包或15s后返回, 单包时长100~200ms最优
type VcASRApp struct {
	wsx *wsx.WSClient

	// 鉴权与配置
	appKey     string
	accessKey  string
	resourceId string
	url        string
	setting    *app.ASRSetting

	// seq 发送的消息序列号
	seq int
	// connId 连接id, 标识一次连接
	connId string
	// logId 服务端返回的logId, 用于定位问题
	logId string
	// session
	uSession, dSession string
	// header 是请求头, 携带鉴权信息
	header http.Header
}

// NewVcASRApp 构造一个新的ASR App
func NewVcASRApp(uSession, appKey, accessKey, resourceId, url string, setting *app.ASRSetting) *VcASRApp {
	logId := genLogID()
	dSession := uuid.New().String()
	asr := &VcASRApp{
		appKey:     appKey,
		accessKey:  accessKey,
		resourceId: resourceId,
		url:        url,
		setting:    setting,
		seq:        1,
		connId:     dSession,
		logId:      logId,
		uSession:   uSession,
		dSession:   dSession,
	}
	asr.buildHTTPHeader()
	return asr
}

// Dial 建立ws链接
func (app *VcASRApp) Dial(ctx context.Context) (err error) {
	ctx = util.NNCtx(ctx)
	app.wsx, err = wsx.NewWSClientWithDial(ctx, app.url, app.header)
	return err
}

// Start 完成应用层协议握手
func (app *VcASRApp) Start() (err error) {
	var payload []byte
	setting := app.setting
	// 协商配置参数
	req := map[string]any{
		// 用户参数
		"user": map[string]any{
			"uid": app.uSession,
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
	seq := util.IntToBytes(app.seq)
	size := util.IntToBytes(len(payload))
	fullClientRequest := util.BuildBytes(header, seq, size, payload)
	if err = app.wsx.WriteBytes(fullClientRequest); err != nil {
		return err
	}
	return
}

// Send 发送音频流
func (app *VcASRApp) Send(ctx context.Context, data []byte) (err error) {
	var payload, header []byte
	ctx = util.NNCtx(ctx)

	// 判断是否最后一个包, 若是则负载为空
	header = negHeader
	if !isLast(data) { // 不是负包则正常处理
		header = posHeader
		payload, err = util.GzipCompress(data)
		if err != nil {
			return err
		}
	}

	// 发送音频流
	app.seq++
	seq := util.IntToBytes(app.seq)
	payloadSize := util.IntToBytes(len(payload))
	audioOnlyRequest := util.BuildBytes(header, seq, payloadSize, payload)
	if err = app.wsx.WriteBytes(audioOnlyRequest); err != nil {
		return err
	}
	return nil
}

// Receive 接受响应
func (app *VcASRApp) Receive(ctx context.Context) (text string, err error) {
	var res []byte
	var mt int
	if mt, res, err = app.wsx.Read(); err == nil {
		switch mt {
		case websocket.BinaryMessage:
			return app.receiveBytes(res)
		case websocket.TextMessage:
			return app.receiveText(res)
		default:
			return "", fmt.Errorf("[volc asr] Receive: invalid websocket message")
		}
	}
	return "", err
}

// receiveText 接受到文本消息, 暂无实际用途
func (app *VcASRApp) receiveText(res []byte) (string, error) {
	logx.Info("[volc asr] receiveText: ", string(res))
	return "", nil
}

// receiveBytes 接收到字节流
func (app *VcASRApp) receiveBytes(res []byte) (text string, err error) {
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
func (app *VcASRApp) Close() (err error) {
	if app.wsx != nil {
		return app.wsx.Close()
	}
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
func (app *VcASRApp) buildHTTPHeader() {
	app.header = http.Header{
		"X-Tt-Logid":        []string{app.logId},
		"X-Api-Resource-Id": []string{app.resourceId},
		"X-Api-Access-Key":  []string{app.accessKey},
		"X-Api-App-Key":     []string{app.appKey},
		"X-Api-Connect-Id":  []string{app.connId},
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

// isLast 判断是否是结束包
func isLast(data []byte) bool {
	return len(data) == 1 && data[0] == lastOne
}
