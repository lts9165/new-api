package service

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// XunhuPayConfig 虎皮椒支付配置
type XunhuPayConfig struct {
	AppID     string // 虎皮椒 APPID
	AppSecret string // 虎皮椒 AppSecret
	Gateway   string // 支付网关地址
}

// XunhuPayRequest 虎皮椒支付请求参数
type XunhuPayRequest struct {
	Version       string `json:"version"`        // API版本号
	AppID         string `json:"appid"`          // APP ID
	TradeOrderID  string `json:"trade_order_id"` // 商户订单号
	TotalFee      string `json:"total_fee"`      // 订单金额(元)
	Title         string `json:"title"`          // 订单标题
	Time          string `json:"time"`           // 时间戳
	NotifyURL     string `json:"notify_url"`     // 异步通知地址
	ReturnURL     string `json:"return_url"`     // 同步跳转地址
	CallbackURL   string `json:"callback_url"`   // 取消支付跳转地址
	Type          string `json:"type"`           // 支付类型: wechat(微信) alipay(支付宝)
	NonceStr      string `json:"nonce_str"`      // 随机字符串
	Hash          string `json:"hash"`           // 签名
}

// XunhuPayResponse 虎皮椒支付响应
type XunhuPayResponse struct {
	OpenID    int64  `json:"openid"`     // 订单ID
	URLQRCode string `json:"url_qrcode"` // 二维码图片URL(PC端扫码)
	URL       string `json:"url"`        // 支付链接(手机端)
	ErrCode   int    `json:"errcode"`    // 错误码
	ErrMsg    string `json:"errmsg"`     // 错误信息
	Hash      string `json:"hash"`       // 响应签名
}

// XunhuPayCallback 虎皮椒支付回调参数
type XunhuPayCallback struct {
	TradeOrderID  string `json:"trade_order_id"`  // 商户订单号
	TotalFee      string `json:"total_fee"`       // 支付金额
	TransactionID string `json:"transaction_id"`  // 平台交易ID
	OpenOrderID   string `json:"open_order_id"`   // 虎皮椒内部订单ID
	OrderTitle    string `json:"order_title"`     // 订单标题
	Status        string `json:"status"`          // 订单状态
	Time          string `json:"time"`            // 时间戳
	Hash          string `json:"hash"`            // 签名
}

// XunhuPayClient 虎皮椒支付客户端
type XunhuPayClient struct {
	config *XunhuPayConfig
}

// NewXunhuPayClient 创建虎皮椒支付客户端
func NewXunhuPayClient(config *XunhuPayConfig) *XunhuPayClient {
	if config.Gateway == "" {
		config.Gateway = "https://api.xunhupay.com/payment/do.html"
	}
	return &XunhuPayClient{
		config: config,
	}
}

// GetXunhuPayClient 获取虎皮椒支付客户端
func GetXunhuPayClient() *XunhuPayClient {
	// 从配置中获取虎皮椒支付配置
	appID := operation_setting.XunhuPayAppID
	appSecret := operation_setting.XunhuPayAppSecret
	gateway := operation_setting.XunhuPayGateway

	if appID == "" || appSecret == "" {
		return nil
	}

	config := &XunhuPayConfig{
		AppID:     appID,
		AppSecret: appSecret,
		Gateway:   gateway,
	}

	return NewXunhuPayClient(config)
}

// GenerateHash 生成签名
// 按照虎皮椒文档要求:
// 1. 将所有非空参数按参数名ASCII码从小到大排序
// 2. 拼接成 key1=value1&key2=value2 格式
// 3. 在字符串末尾拼接上AppSecret
// 4. 对结果进行MD5运算,转小写
func (c *XunhuPayClient) GenerateHash(params map[string]string) string {
	// 获取所有key并排序
	keys := make([]string, 0, len(params))
	for key := range params {
		// hash字段不参与签名
		if key == "hash" || params[key] == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// 拼接字符串
	var sb strings.Builder
	for i, key := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(key)
		sb.WriteString("=")
		sb.WriteString(params[key])
	}

	// 拼接AppSecret
	sb.WriteString(c.config.AppSecret)

	// MD5加密并转小写
	hash := md5.Sum([]byte(sb.String()))
	return strings.ToLower(hex.EncodeToString(hash[:]))
}

// VerifyHash 验证签名
func (c *XunhuPayClient) VerifyHash(params map[string]string, receivedHash string) bool {
	calculatedHash := c.GenerateHash(params)
	return calculatedHash == receivedHash
}

// CreateOrder 创建支付订单
func (c *XunhuPayClient) CreateOrder(
	tradeOrderID string,
	totalFee float64,
	title string,
	paymentType string, // "wechat" 或 "alipay"
	notifyURL string,
	returnURL string,
) (*XunhuPayResponse, error) {
	// 生成随机字符串
	nonceStr := common.GetRandomString(16)

	// 构建请求参数
	params := map[string]string{
		"version":        "1.1",
		"appid":          c.config.AppID,
		"trade_order_id": tradeOrderID,
		"total_fee":      fmt.Sprintf("%.2f", totalFee),
		"title":          title,
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"notify_url":     notifyURL,
		"return_url":     returnURL,
		"callback_url":   returnURL,
		"type":           paymentType,
		"nonce_str":      nonceStr,
	}

	// 生成签名
	params["hash"] = c.GenerateHash(params)

	// 构建POST请求
	formData := url.Values{}
	for key, value := range params {
		formData.Set(key, value)
	}

	// 发送请求
	resp, err := http.PostForm(c.config.Gateway, formData)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析响应
	var payResp XunhuPayResponse
	if err := json.Unmarshal(body, &payResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, body: %s", err, string(body))
	}

	// 检查错误码
	if payResp.ErrCode != 0 {
		return nil, fmt.Errorf("虎皮椒返回错误: %s (code: %d)", payResp.ErrMsg, payResp.ErrCode)
	}

	// 验证响应签名(可选,增强安全性)
	respParams := map[string]string{
		"openid":     strconv.FormatInt(payResp.OpenID, 10),
		"url_qrcode": payResp.URLQRCode,
		"url":        payResp.URL,
		"errcode":    strconv.Itoa(payResp.ErrCode),
		"errmsg":     payResp.ErrMsg,
	}
	if !c.VerifyHash(respParams, payResp.Hash) {
		return nil, fmt.Errorf("响应签名验证失败")
	}

	return &payResp, nil
}

// VerifyCallback 验证回调签名
func (c *XunhuPayClient) VerifyCallback(callback map[string]string) bool {
	hash := callback["hash"]
	if hash == "" {
		return false
	}

	// 移除hash字段后验证
	delete(callback, "hash")
	return c.VerifyHash(callback, hash)
}
