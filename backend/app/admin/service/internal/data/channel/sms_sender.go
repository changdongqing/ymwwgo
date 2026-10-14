package channel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	notificationV1 "go-wind-admin/api/gen/go/notification/service/v1"
	notificationChannelV1 "go-wind-admin/api/gen/go/notification_channel/service/v1"

	"go-wind-admin/app/admin/service/internal/data"
)

// smsTimeout 单次出站请求的总超时（拨号 + 写请求 + 读响应）。
// 与 webhookTimeout 同形：出站这件事不需要每个渠道各定一个数。
const smsTimeout = 10 * time.Second

// 短信服务商标识。与 sys_notification_channels.sms_provider 列、proto 的 SmsProvider
// 成员名逐字相同（repo 的 EnumTypeConverter 按名字字符串配对），这里只做 sender 内部分派。
const (
	smsProviderAliyun  = "ALIYUN"
	smsProviderTencent = "TENCENT"
)

// SmsSender 短信渠道（阿里云 / 腾讯云）。
//
// 与 EmailSender/WebhookSender 共用同一套渠道选择策略（显式 channel_id 优先，
// 否则取第一个启用的 SMS 行），因此"配了两条短信渠道会走哪条"只有一个答案。
//
// 短信与邮件/webhook 的本质差异：**系统侧没有自由文本通道**。三家服务商都要求
// 正文出自审核通过的模板，验证码作为模板占位值传入——所以 Send 只接受 params["code"]，
// 没有它（比如告警事件被路由到 SMS）直接失败并把原因说清楚，而不是发一条没有验证码的短信。
// 出站地址是服务商的固定域名（不是渠道行里配的 URL），SSRF 拦网在这里没有对象。
type SmsSender struct {
	channel *data.NotificationChannelRepo
	client  *http.Client
}

var _ Prechecker = (*SmsSender)(nil)

func NewSmsSender(channelRepo *data.NotificationChannelRepo) *SmsSender {
	return &SmsSender{
		channel: channelRepo,
		client:  &http.Client{Timeout: smsTimeout},
	}
}

// Channel 渠道标识。
func (s *SmsSender) Channel() notificationV1.Channel {
	return notificationV1.Channel_SMS
}

// Precheck 只解析渠道配置并自检必填项，不拨号。见 channel.Prechecker。
//
// 与 WebhookSender.Precheck 同一条理由：一次性验证码躺在队列里等三次重试才失败，
// 和压根没配渠道一样是没发出去，但台账会记成 FAILED 且验证码已经过期。
func (s *SmsSender) Precheck(ctx context.Context, channelID uint32) error {
	account, err := s.pickAccount(ctx, channelID)
	if err != nil {
		return err
	}
	if err = validateSmsAccount(account); err != nil {
		return fmt.Errorf("%w: channel [%d] %v", ErrChannelNotConfigured, account.ID, err)
	}
	return nil
}

// Send 投递一条验证码短信。返回 error 一律是"投递失败"（含渠道不可用），
// 由服务层用 ErrChannelNotConfigured 判别 SKIPPED / FAILED。
func (s *SmsSender) Send(ctx context.Context, req *SendRequest) (*SendReceipt, error) {
	if req == nil || req.Target == "" {
		return nil, errors.New("sms target is empty")
	}

	// 没带 code 的投递在选账号之前就拒绝：这是路由/调用方的问题，不是渠道配置问题，
	// 记 FAILED（下一次路由改对了就好了），不该混进 SKIPPED 让管理员白查一遍配置。
	code := req.Params["code"]
	if code == "" {
		return nil, fmt.Errorf(`sms channel [%d] requires params["code"]: SMS only carries verification-code events, the body is rendered by the provider-side approved template`, req.ChannelID)
	}

	account, err := s.pickAccount(ctx, req.ChannelID)
	if err != nil {
		return nil, err
	}

	national, e164, err := NormalizePhoneNumber(req.Target)
	if err != nil {
		return &SendReceipt{ChannelID: account.ID}, fmt.Errorf("%w: %v", ErrChannelNotConfigured, err)
	}

	if err = sendSmsCode(ctx, s.client, account, national, e164, code); err != nil {
		// 账号已选出：失败也必须把"是哪条渠道没发出去"带回给台账（见 Sender 的契约）。
		return &SendReceipt{ChannelID: account.ID}, fmt.Errorf("send sms via channel [%d] failed: %w", account.ID, err)
	}

	return &SendReceipt{ChannelID: account.ID}, nil
}

// pickAccount 解析实际使用的短信账号：显式 ID 优先，并校验类型与启用状态。
// 与 EmailSender.pickAccount 同一条判据：一切"渠道用不了"都包成 ErrChannelNotConfigured，
// 服务层据此把台账记 SKIPPED（没拨过号）而不是 FAILED（拨过号被拒）。
func (s *SmsSender) pickAccount(ctx context.Context, channelID uint32) (*data.SmsAccount, error) {
	if channelID == 0 {
		account, err := s.channel.GetFirstEnabledSmsChannel(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrChannelNotConfigured, err)
		}
		return account, nil
	}

	// 先走脱敏的 Get 校验类型/启用：GetDecryptedSmsAccount 守类型但不看 status，
	// 直接取会把一条停用的渠道当可用配置用。
	dto, err := s.channel.Get(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("%w: channel [%d] not found: %v", ErrChannelNotConfigured, channelID, err)
	}
	if dto.GetType() != notificationChannelV1.NotificationChannel_SMS {
		return nil, fmt.Errorf("%w: channel [%d] is not an SMS channel", ErrChannelNotConfigured, channelID)
	}
	if !dto.GetEnabled() {
		return nil, fmt.Errorf("%w: channel [%d] is disabled", ErrChannelNotConfigured, channelID)
	}

	account, err := s.channel.GetDecryptedSmsAccount(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("%w: channel [%d] credentials: %v", ErrChannelNotConfigured, channelID, err)
	}

	return account, nil
}

// validateSmsAccount 按服务商核对必填配置：两家差在腾讯云多要 app id 与地域。
// 在 Precheck 拦住缺项，错误文案直接点名缺哪个字段；用切片保序，缺多项时报第一个。
func validateSmsAccount(account *data.SmsAccount) error {
	var required []struct{ name, value string }
	switch account.Provider {
	case smsProviderAliyun:
		required = []struct{ name, value string }{
			{"sms_access_key_id", account.AccessKeyID},
			{"sms_access_secret", account.AccessSecret},
			{"sms_sign_name", account.SignName},
			{"sms_template_code", account.TemplateCode},
		}
	case smsProviderTencent:
		required = []struct{ name, value string }{
			{"sms_access_key_id", account.AccessKeyID},
			{"sms_access_secret", account.AccessSecret},
			{"sms_sign_name", account.SignName},
			{"sms_template_code", account.TemplateCode},
			{"sms_app_id", account.AppID},
			{"sms_region", account.Region},
		}
	default:
		return fmt.Errorf("unknown sms provider %q (expected ALIYUN / TENCENT)", account.Provider)
	}
	for _, item := range required {
		if item.value == "" {
			return fmt.Errorf("%s is required for provider %s", item.name, account.Provider)
		}
	}
	return nil
}

// sendSmsCode 按服务商分派一次验证码下发。
// national（无 +86 前缀）与 e164（带 +）两形都给：阿里云国内短信收裸号，
// 腾讯云PhoneNumberSet只认 E.164。
func sendSmsCode(ctx context.Context, client *http.Client, account *data.SmsAccount, national, e164, code string) error {
	switch account.Provider {
	case smsProviderAliyun:
		return sendAliyunSms(ctx, client, account, national, code)
	case smsProviderTencent:
		return sendTencentSms(ctx, client, account, e164, code)
	default:
		return fmt.Errorf("unknown sms provider %q (expected ALIYUN / TENCENT)", account.Provider)
	}
}

// NormalizePhoneNumber 归一手机号为国内裸号与 E.164 两形，供绑定入口与 sender 共用
// 同一套判据（服务层用它做快反馈，sender 在投递前再跑一遍作最终裁决）。
//
// 接受两类输入：E.164（可带 +86）与中国大陆 11 位裸号（1 开头，自动补 +86）。
// 其他形态一律拒绝——短信目标错了没有"发到别家去"的软失败，宁可让这一次投递
// 失败在台账里说清楚。分隔符（空格/横线/括号）先剥掉，用户从通讯录粘出来的号常带。
func NormalizePhoneNumber(raw string) (national, e164 string, err error) {
	cleaned := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == ' ' || c == '-' || c == '(' || c == ')' {
			continue
		}
		cleaned = append(cleaned, c)
	}
	s := string(cleaned)
	if s == "" {
		return "", "", fmt.Errorf("phone number %q is empty", raw)
	}

	// 位数校验对去掉前导 + 的主体做：+ 是格式记号，不是数字。
	body := strings.TrimPrefix(s, "+")
	digitCount := 0
	for i := 0; i < len(body); i++ {
		if body[i] >= '0' && body[i] <= '9' {
			digitCount++
		}
	}
	if digitCount != len(body) || digitCount < 5 || digitCount > 20 {
		return "", "", fmt.Errorf("phone number %q is not a valid MSISDN (5-20 digits, optional +86 prefix)", raw)
	}

	switch {
	case strings.HasPrefix(s, "+86"):
		return s[3:], s, nil
	case s[0] == '+':
		return s[1:], s, nil
	case digitCount == 11 && s[0] == '1':
		return s, "+86" + s, nil
	default:
		return "", "", fmt.Errorf("phone number %q must be E.164 (with country code) or a CN mobile number", raw)
	}
}
