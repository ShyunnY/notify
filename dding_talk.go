package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	// dding talk api
	apiUrl = "https://oapi.dingtalk.com/robot/send?access_token="

	// MessageType
	textType     = "text"
	markdownType = "markdown"
	msgType      = "msgtype"

	// AtType
	at        = "at"
	atMobiles = "atMobiles"
	atUserIds = "atUserIds"
	atAll     = "isAtAll"
)

type Robot struct {
	AccessToken string
	Secret      string
	data        Message
	at          *AtPeople
	zlog        *zap.Logger
}

type Message map[string]interface{}
type SendMsgType func(*Robot)

// NewRobot 新建机器人
func NewRobot(accessToken, secret string) *Robot {

	prod, err := logInit()
	if err != nil {
		log.Fatalln("log init error: ", err)
	}

	return &Robot{
		AccessToken: accessToken,
		Secret:      secret,
		data:        make(Message),
		at:          new(AtPeople),
		zlog:        prod,
	}
}

func logInit() (*zap.Logger, error) {
	encConfig := zap.NewProductionEncoderConfig()
	encConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	config := zap.NewProductionConfig()
	config.EncoderConfig = encConfig
	prod, err := config.Build()
	return prod, err
}

// BuildMsgAndSend 构建信息
func (r *Robot) BuildMsgAndSend(smt SendMsgType, opts ...AtOption) error {

	smt(r)
	for _, opt := range opts {
		opt(r.at)
	}

	return r.send(r.data)
}

// send the notification to dding talk
func (r *Robot) send(msg map[string]interface{}) error {

	// build query param
	url := r.buildQuery()

	msg[at] = r.at.serialize()
	reqData, err := sonic.Marshal(msg)
	if err != nil {
		r.zlog.Error("sonic serialization failed for data", zap.Error(err))
		return err
	}

	// send request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqData))
	clear(r.data)
	r.at = new(AtPeople)
	if err != nil {
		r.zlog.Error("dding talk api call failed", zap.Error(err))
		return err
	}

	// 获取钉钉响应
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		r.zlog.Error("read dding talk response error", zap.Error(err))
		return err
	}

	result := new(Response)
	_ = sonic.Unmarshal(respData, result)
	if result.ErrCode != 0 {
		r.zlog.Error(result.Error())
		return result
	}

	r.zlog.Info("message was successfully send to dding talk")
	return nil
}

func (r *Robot) buildQuery() string {
	timestamp := time.Now().UnixMilli()
	signature := fmt.Sprintf("%d\n%s", timestamp, r.Secret)

	// hmac encrypt
	hash := hmac.New(sha256.New, []byte(r.Secret))
	hash.Write([]byte(signature))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	// concat query param
	webhook := apiUrl + r.AccessToken
	url := fmt.Sprintf("%s&timestamp=%d&sign=%s", webhook, timestamp, sign)
	return url
}

// TextType
// used for send text notification
func TextType(content string) SendMsgType {
	return func(r *Robot) {
		r.data[msgType] = textType
		r.data[textType] = map[string]string{
			"content": content,
		}
	}
}

// MarkDownType
// used for send markdown notification
func MarkDownType(title, text string) SendMsgType {
	return func(r *Robot) {
		r.data[msgType] = markdownType
		r.data[markdownType] = map[string]string{
			"title": title,
			"text":  text,
		}
	}
}

type AtPeople struct {
	atMobiles []string
	atUserIds []string
	isAtAll   bool
}

func (at *AtPeople) serialize() map[string]interface{} {
	ret := map[string]interface{}{}

	if len(at.atMobiles) > 0 {
		ret[atMobiles] = at.atMobiles
	}

	if len(at.atUserIds) > 0 {
		ret[atUserIds] = at.atUserIds
	}

	if at.isAtAll {
		ret[atAll] = at.isAtAll
	}

	return ret
}

type AtOption func(people *AtPeople)

func WithAtMobiles(mobiles ...string) AtOption {
	return func(p *AtPeople) {
		p.atMobiles = mobiles
	}
}

func WithAtUserIds(userIds ...string) AtOption {
	return func(p *AtPeople) {
		p.atUserIds = userIds
	}
}

func WithAtAll() AtOption {
	return func(p *AtPeople) {
		p.isAtAll = true
	}
}

type Response struct {
	ErrCode int
	ErrMsg  string
}

func (r Response) Error() string {
	return fmt.Sprintf("dding talk response info: errcode=%d,errmsg=%s", r.ErrCode, r.ErrMsg)
}
