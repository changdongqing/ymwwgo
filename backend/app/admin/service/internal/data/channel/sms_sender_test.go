// SMS 渠道的仓库侧测试。
//
// 服务商签名向量、百分号编码、手机号归一与按服务商的必填项自检已随实现
// 下沉 go-utils/notify/sms 并在那里测试；这里只留仓库侧的业务策略：
// SMS 只发验证码事件（params["code"]），没有它的投递在选账号之前就拒绝。
package channel

import (
	"context"
	"strings"
	"testing"
)

// TestSmsSendRequiresCodeParam 没带 params["code"] 的投递在选账号之前就被拒绝：
// 这是路由/调用方的问题，不该混进"渠道未配置"让管理员白查配置页。
func TestSmsSendRequiresCodeParam(t *testing.T) {
	sender := NewSmsSender(nil)
	_, err := sender.Send(context.Background(), &SendRequest{Target: "13800138000", Title: "t", Content: "c"})
	if err == nil || !strings.Contains(err.Error(), `params["code"]`) {
		t.Fatalf("Send without code should name params[\"code\"], got %v", err)
	}
}
