package channel

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"go-wind-admin/app/admin/service/internal/data"
)

// TestSignAliyunRPC 阿里云 RPC 签名的钉值与性质测试。
//
// 钉值锁的是"参数排序 + 百分号编码 + HMAC-SHA1(secret&)"这一整条链：
// 任何一环被改（比如编码换成 QueryEscape 原样），签名就全量漂移，对端一律报 sign not match。
func TestSignAliyunRPC(t *testing.T) {
	params := url.Values{}
	params.Set("AccessKeyId", "testkey")
	params.Set("Action", "SendSms")
	params.Set("Format", "JSON")
	params.Set("PhoneNumbers", "13800138000")
	params.Set("RegionId", "cn-hangzhou")
	params.Set("SignName", "签名测试")
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureNonce", "nonce-1")
	params.Set("SignatureVersion", "1.0")
	params.Set("TemplateCode", "SMS_1234567")
	params.Set("TemplateParam", `{"code":"123456"}`)
	params.Set("Timestamp", "2026-10-15T06:00:00Z")
	params.Set("Version", "2017-05-26")

	const want = "gS9g6UDJP7wPmNOLjjv07DkaWNk="
	if got := signAliyunRPC(params, "testsecret"); got != want {
		t.Fatalf("signAliyunRPC() = %q, want %q", got, want)
	}

	// 拼装顺序无关性：canonical query 按参数名排序，调用方先 Set 谁不该影响结果。
	shuffled := url.Values{}
	for _, k := range []string{"Version", "Timestamp", "TemplateParam", "TemplateCode", "SignatureVersion",
		"SignatureNonce", "SignatureMethod", "SignName", "RegionId", "PhoneNumbers", "Format", "Action", "AccessKeyId"} {
		shuffled.Set(k, params.Get(k))
	}
	if got := signAliyunRPC(shuffled, "testsecret"); got != want {
		t.Fatalf("signAliyunRPC() with shuffled insertion order = %q, want stable %q", got, want)
	}
}

// TestAliyunPercentEncode RFC3986 与 QueryEscape 的三处分歧各钉一条。
func TestAliyunPercentEncode(t *testing.T) {
	cases := map[string]string{
		"a b":   "a%20b", // 空格：%20，不是 +
		"a*b":   "a%2Ab", // 星号必须转义（QueryEscape 放过它）
		"a~b":   "a~b",   // 波浪号不转义（QueryEscape 本就不动它，钉住防回归）
		"签名":    "%E7%AD%BE%E5%90%8D",
		`{"x"}`: "%7B%22x%22%7D",
	}
	for in, want := range cases {
		if got := aliyunPercentEncode(in); got != want {
			t.Fatalf("aliyunPercentEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSignTencentTC3 TC3 签名的钉值与敏感性测试。
// 钉值含 Credential 段的日期派生（从时间戳取 UTC 日期）与整条四段 HMAC 链。
func TestSignTencentTC3(t *testing.T) {
	now := time.Unix(1760508000, 0).UTC() // 2025-10-15
	payload := []byte(`{"PhoneNumberSet":["+8613800138000"]}`)

	const want = "TC3-HMAC-SHA256 Credential=AKIDtest/2025-10-15/sms/tc3_request, SignedHeaders=content-type;host, " +
		"Signature=87a2b3a99f4de4ff56c14d845602b819bab7e2dc4e3413e5edc030c144e5195e"
	if got := signTencentTC3("AKIDtest", "testsecret", now, payload); got != want {
		t.Fatalf("signTencentTC3() = %q, want %q", got, want)
	}

	// 签名盖住请求体：body 变一个字节，Signature 段必须变（Credential 段不变）。
	other := signTencentTC3("AKIDtest", "testsecret", now, []byte(`{"PhoneNumberSet":["+8613800138001"]}`))
	if other == want {
		t.Fatal("signTencentTC3() identical for different payloads")
	}
	if !strings.HasPrefix(other, "TC3-HMAC-SHA256 Credential=AKIDtest/2025-10-15/sms/tc3_request") {
		t.Fatalf("credential/date prefix drifted: %q", other)
	}

	// 密钥错了签名必须不同：这是"配错 Secret 会对端拒签"这条排障路径的根基。
	if wrongSecret := signTencentTC3("AKIDtest", "othersecret", now, payload); wrongSecret == want {
		t.Fatal("signTencentTC3() identical for different secrets")
	}
}

// TestNormalizePhoneNumber 两种接受形态、分隔符剥离与拒绝形态。
func TestNormalizePhoneNumber(t *testing.T) {
	cases := []struct {
		raw      string
		national string
		e164     string
		wantErr  bool
	}{
		{raw: "13800138000", national: "13800138000", e164: "+8613800138000"},
		{raw: "138 0013-8000", national: "13800138000", e164: "+8613800138000"}, // 通讯录粘贴形态
		{raw: "+8613800138000", national: "13800138000", e164: "+8613800138000"},
		{raw: "+12025550123", national: "12025550123", e164: "+12025550123"}, // 非中国号：原样两形
		{raw: "12345", wantErr: true},                        // 裸 5 位：既不是 E.164 也不是 CN 手机
		{raw: "1380013800a", wantErr: true},                  // 混入字母
		{raw: "13800138000112345678", wantErr: true},         // 超 20 位
		{raw: "", wantErr: true},
	}
	for _, tc := range cases {
		national, e164, err := NormalizePhoneNumber(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizePhoneNumber(%q) expected error, got (%q, %q)", tc.raw, national, e164)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizePhoneNumber(%q) unexpected error: %v", tc.raw, err)
		}
		if national != tc.national || e164 != tc.e164 {
			t.Fatalf("NormalizePhoneNumber(%q) = (%q, %q), want (%q, %q)", tc.raw, national, e164, tc.national, tc.e164)
		}
	}
}

// TestValidateSmsAccount 按服务商核对必填项：缺哪项报哪项，未知厂商直接拒绝。
func TestValidateSmsAccount(t *testing.T) {
	full := func() *data.SmsAccount {
		return &data.SmsAccount{
			Provider: smsProviderAliyun, AccessKeyID: "k", AccessSecret: "s",
			SignName: "sig", TemplateCode: "SMS_1", AppID: "140000", Region: "ap-guangzhou",
		}
	}

	if err := validateSmsAccount(full()); err != nil {
		t.Fatalf("validateSmsAccount(full aliyun) unexpected error: %v", err)
	}
	if err := validateSmsAccount(full()); err != nil {
		t.Fatalf("validateSmsAccount(full tencent) unexpected error: %v", err)
	}

	tencent := full()
	tencent.Provider = smsProviderTencent
	tencent.AppID = ""
	if err := validateSmsAccount(tencent); err == nil || !strings.Contains(err.Error(), "sms_app_id") {
		t.Fatalf("tencent without app id should name sms_app_id, got %v", err)
	}
	tencent.AppID = "140000"
	tencent.Region = ""
	if err := validateSmsAccount(tencent); err == nil || !strings.Contains(err.Error(), "sms_region") {
		t.Fatalf("tencent without region should name sms_region, got %v", err)
	}

	aliyun := full()
	aliyun.AppID, aliyun.Region = "", "" // 阿里云不需要这两项
	if err := validateSmsAccount(aliyun); err != nil {
		t.Fatalf("aliyun without app id/region should pass, got %v", err)
	}

	if err := validateSmsAccount(&data.SmsAccount{Provider: "SMSC"}); err == nil || !strings.Contains(err.Error(), "unknown sms provider") {
		t.Fatalf("unknown provider should be rejected, got %v", err)
	}
}

// TestSmsSendRequiresCodeParam 没带 params["code"] 的投递在选账号之前就被拒绝：
// 这是路由/调用方的问题，不该混进"渠道未配置"让管理员白查配置页。
func TestSmsSendRequiresCodeParam(t *testing.T) {
	sender := NewSmsSender(nil)
	_, err := sender.Send(context.Background(), &SendRequest{Target: "13800138000", Title: "t", Content: "c"})
	if err == nil || !strings.Contains(err.Error(), `params["code"]`) {
		t.Fatalf("Send without code should name params[\"code\"], got %v", err)
	}
}
