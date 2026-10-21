package constants

// sys_config 平台参数键与默认值：登录链路开关。
// 参数经参数管理页（sys_configs，is_built_in）可改，修改落库后
// 经 ConfigRepo 本地失效 + Redis 广播即时生效（多实例无需重启）。
const (
	// ConfigKeyLoginCaptchaEnabled 密码登录是否强制校验图形验证码。
	// 默认 DefaultLoginCaptchaEnabled（开启）；关闭后登录页不再展示验证码，
	// 但登录限流与登录策略闸门仍然生效。
	ConfigKeyLoginCaptchaEnabled = "sys.login.captchaEnabled"
)

const (
	// DefaultLoginCaptchaEnabled 验证码默认开启：安全优先，且与既有部署行为一致。
	DefaultLoginCaptchaEnabled = true
)
