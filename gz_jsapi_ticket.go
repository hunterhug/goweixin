package goweixin

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hunterhug/marmot/miner"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GzClient struct {
	AppId             string
	AppSecret         string
	AccessToken       string
	AccessTokenExpire int64
	JsapiTicket       string
	JsapiTicketExpire int64
	accessTokenLock   sync.Mutex
	jsapiTicketLock   sync.Mutex
}

func NewGzClient(appId, appSecret string) *GzClient {
	m := new(GzClient)
	m.AppId = appId
	m.AppSecret = appSecret
	return m
}

type JsapiTicket struct {
	Ticket string `json:"ticket"`
}

type JsapiTicketSign struct {
	AppId     string `json:"appId"`
	NonceStr  string `json:"nonceStr"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
}

// GetJsapiTicketAndSign JSSDK使用步骤: https://developers.weixin.qq.com/doc/offiaccount/OA_Web_Apps/JS-SDK.html#3
// 签名说明：https://developers.weixin.qq.com/doc/offiaccount/OA_Web_Apps/JS-SDK.html#62
func (c *GzClient) GetJsapiTicketAndSign(signUrl string) (ticket string, ticketSign *JsapiTicketSign, err error) {
	c.jsapiTicketLock.Lock()
	defer c.jsapiTicketLock.Unlock()

	if c.JsapiTicket != "" && c.JsapiTicketExpire <= time.Now().Unix() {
		ticketSign, err = c.signJsapiTicket(c.JsapiTicket, signUrl)
		if err != nil {
			return "", nil, err
		}
		return c.JsapiTicket, ticketSign, nil
	}

	token, err := c.AuthGetAccessToken()
	if err != nil {
		return "", nil, err
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/ticket/getticket?access_token=%s&type=jsapi", token)
	api := miner.NewAPI()
	raw, err := api.Clone().SetUrl(url).Get()
	if err != nil {
		return "", nil, err
	}

	miner.Logger.Infof("GzClient GetJsapiTicketAndSign: %v", string(raw))

	wErr := new(ErrorRsp)
	err = json.Unmarshal(raw, wErr)
	if err != nil {
		return "", nil, err
	}

	if wErr.ErrCode != 0 {
		if strings.Contains(wErr.ErrMsg, "access_token expired") {
			c.AccessToken = ""
		}
		return "", nil, wErr
	}

	t := new(JsapiTicket)
	err = json.Unmarshal(raw, t)
	if err != nil {
		return "", nil, err
	}

	if t.Ticket == "" {
		return "", nil, errors.New("ticket empty")
	}

	c.JsapiTicket = t.Ticket
	c.JsapiTicketExpire = time.Now().Unix() + 3600

	ticketSign, err = c.signJsapiTicket(t.Ticket, signUrl)
	if err != nil {
		return "", nil, err
	}

	return t.Ticket, ticketSign, nil
}

func nonceStr() string {
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func (c *GzClient) signJsapiTicket(ticket, signUrl string) (*JsapiTicketSign, error) {
	nonce := nonceStr()
	timestamp := time.Now().Unix()
	s := new(JsapiTicketSign)
	s.NonceStr = nonce
	s.AppId = c.AppId
	s.Timestamp = timestamp

	str := "jsapi_ticket=" + ticket + "&noncestr=" + nonce + "&timestamp=" + fmt.Sprintf("%d", timestamp) + "&url=" + signUrl
	h := sha1.New()
	_, err := h.Write([]byte(str))
	if err != nil {
		return nil, err
	}

	s.Signature = fmt.Sprintf("%x", h.Sum(nil))
	return s, nil
}

type GzAccessToken struct {
	AccessToken string `json:"access_token"`
}

// AuthGetAccessToken https://developers.weixin.qq.com/miniprogram/dev/api-backend/open-api/access-token/auth.getAccessToken.html
// https://developers.weixin.qq.com/doc/offiaccount/Basic_Information/Get_access_token.html
func (c *GzClient) AuthGetAccessToken() (token string, err error) {
	c.accessTokenLock.Lock()
	defer c.accessTokenLock.Unlock()

	if c.AccessToken != "" && c.AccessTokenExpire <= time.Now().Unix() {
		return c.AccessToken, nil
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", c.AppId, c.AppSecret)
	api := miner.NewAPI()
	raw, err := api.Clone().SetUrl(url).Get()
	if err != nil {
		return "", err
	}

	miner.Logger.Infof("GzClient AuthGetAccessToken token: %v", string(raw))

	wErr := new(ErrorRsp)
	err = json.Unmarshal(raw, wErr)
	if err != nil {
		return "", err
	}

	if wErr.ErrCode != 0 {
		return "", wErr
	}

	t := new(GzAccessToken)
	err = json.Unmarshal(raw, t)
	if err != nil {
		return "", err
	}

	if t.AccessToken == "" {
		return "", errors.New("token empty")
	}

	c.AccessToken = t.AccessToken
	c.AccessTokenExpire = time.Now().Unix() + 1800
	return t.AccessToken, nil
}
