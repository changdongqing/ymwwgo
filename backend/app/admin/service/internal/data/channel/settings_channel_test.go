package channel

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSettingsCatalogConsistency 声明与构造器一一对应：
// 每个 schema 有 builder、每个 builder 有 schema，数量即渠道清单（39）。
func TestSettingsCatalogConsistency(t *testing.T) {
	if len(settingsSchemas) == 0 {
		t.Fatal("no settings schemas registered")
	}
	for _, s := range settingsSchemas {
		if _, ok := settingsBuilders[s.Type]; !ok {
			t.Errorf("schema %q has no builder", s.Type)
		}
		if s.Group == "" || s.Label == "" {
			t.Errorf("schema %q missing group/label", s.Type)
		}
		for _, f := range s.Fields {
			switch f.Type {
			case "text", "textarea", "number", "password", "select":
			default:
				t.Errorf("schema %q field %q has unknown type %q", s.Type, f.Name, f.Type)
			}
			if f.Sensitive && f.Type != "password" {
				t.Errorf("schema %q field %q is sensitive but type is %q (should be password)", s.Type, f.Name, f.Type)
			}
		}
	}
	if len(settingsBuilders) != len(settingsSchemas) {
		t.Fatalf("builders (%d) and schemas (%d) diverge", len(settingsBuilders), len(settingsSchemas))
	}
	if got := len(SettingsTypeNames()); got != len(settingsSchemas) {
		t.Fatalf("SettingsTypeNames() = %d, want %d", got, len(settingsSchemas))
	}
}

// TestBuildAllSettingsChannels 每个渠道用"必填字段全填 dummy"的最小配置构造一次：
// 构造必须成功且返回非 nil Service（协议错误在 Send 时由库报，构造层不该炸）。
func TestBuildAllSettingsChannels(t *testing.T) {
	for _, schema := range settingsSchemas {
		settings := map[string]any{}
		for _, f := range schema.Fields {
			switch f.Type {
			case "number":
				settings[f.Name] = 1
			case "select":
				if len(f.Options) > 0 {
					settings[f.Name] = f.Options[0]
				} else {
					settings[f.Name] = ""
				}
			default:
				settings[f.Name] = "test-" + f.Name
			}
		}
		service, err := BuildSettingsService(schema.Type, settings)
		if err != nil {
			t.Errorf("build %s: %v", schema.Type, err)
			continue
		}
		if service == nil {
			t.Errorf("build %s returned nil service", schema.Type)
			continue
		}
		if service.Name() == "" {
			t.Errorf("build %s returned empty service name", schema.Type)
		}
	}
}

// TestBuildUnknownTypeFails 未知类型必须报错（路由指到没注册的扩展渠道是代码 bug）。
func TestBuildUnknownTypeFails(t *testing.T) {
	if _, err := BuildSettingsService("NOT_A_CHANNEL", map[string]any{}); err == nil {
		t.Fatal("unknown type should fail")
	}
}

// TestNormalizeSettings 加密落库闭环：
// required 校验拦空值、sensitive 字段密文落库（可解回）、非敏感字段明文保留。
func TestNormalizeSettings(t *testing.T) {
	const typeName = "TELEGRAM" // botToken(sensitive) + apiBase + parseMode
	plaintext := `{"botToken":"123:ABC","apiBase":"https://example.com","parseMode":"HTML"}`

	storeJSON, err := NormalizeSettingsForStore(typeName, plaintext)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	var stored map[string]any
	if err = json.Unmarshal([]byte(storeJSON), &stored); err != nil {
		t.Fatalf("stored json: %v", err)
	}
	if stored["apiBase"] != "https://example.com" {
		t.Fatalf("plain field drifted: %v", stored["apiBase"])
	}
	// GOWIND_CRYPTO_KEY 未配置时按约定降级明文存储（dev 环境），两种形态都合法
	botToken, _ := stored["botToken"].(string)
	if botToken == "" {
		t.Fatalf("sensitive field must be stored, got empty")
	}

	decrypted, err := DecryptSettings(typeName, storeJSON)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted["botToken"] != "123:ABC" {
		t.Fatalf("decrypt round-trip failed: %v", decrypted["botToken"])
	}

	// 回显脱敏：sensitive 恒空串，非敏感保留
	sanitized := SanitizeSettingsForDTO(typeName, storeJSON)
	var echo map[string]any
	if err = json.Unmarshal([]byte(sanitized), &echo); err != nil {
		t.Fatalf("sanitized json: %v", err)
	}
	if echo["botToken"] != "" {
		t.Fatalf("sensitive field must be masked in DTO, got %v", echo["botToken"])
	}
	if echo["apiBase"] != "https://example.com" {
		t.Fatalf("plain field must survive sanitize: %v", echo["apiBase"])
	}
}

// TestNormalizeSettingsRequired 校验 required：缺必填在保存期就被拒（不等到发送）。
func TestNormalizeSettingsRequired(t *testing.T) {
	_, err := NormalizeSettingsForStore("TELEGRAM", `{"apiBase":"https://example.com"}`)
	if err == nil || !strings.Contains(err.Error(), "Bot Token") {
		t.Fatalf("missing required should name the field, got %v", err)
	}
	if _, err = NormalizeSettingsForStore("TELEGRAM", "not-json"); err == nil {
		t.Fatal("invalid JSON should fail")
	}
}

// TestMergeSettings 敏感字段留空 = 保留已存密文（回显脱敏后的表单回提交不丢密钥）。
func TestMergeSettings(t *testing.T) {
	const typeName = "TELEGRAM"
	storeJSON, err := NormalizeSettingsForStore(typeName, `{"botToken":"123:ABC","apiBase":"https://old.example.com"}`)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	// 回显脱敏版（botToken 为空串）回提交：botToken 必须保留旧密文，apiBase 更新
	echoJSON := SanitizeSettingsForDTO(typeName, storeJSON)
	merged, err := MergeSettingsForStore(typeName, storeJSON, echoJSON)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	decrypted, err := DecryptSettings(typeName, merged)
	if err != nil {
		t.Fatalf("decrypt merged: %v", err)
	}
	if decrypted["botToken"] != "123:ABC" {
		t.Fatalf("merge lost the stored secret: %v", decrypted["botToken"])
	}
	if decrypted["apiBase"] != "https://old.example.com" {
		t.Fatalf("merge should keep untouched plain fields: %v", decrypted["apiBase"])
	}

	// 敏感字段给了新值则用新值
	merged2, err := MergeSettingsForStore(typeName, storeJSON, `{"botToken":"999:NEW","apiBase":"https://new.example.com"}`)
	if err != nil {
		t.Fatalf("merge2: %v", err)
	}
	decrypted2, _ := DecryptSettings(typeName, merged2)
	if decrypted2["botToken"] != "999:NEW" {
		t.Fatalf("merge should accept new secret: %v", decrypted2["botToken"])
	}
	if decrypted2["apiBase"] != "https://new.example.com" {
		t.Fatalf("merge should accept new plain value: %v", decrypted2["apiBase"])
	}
}

// TestValidateSettingsRequired Precheck 的必填校验（缺一项点一项的名）。
func TestValidateSettingsRequired(t *testing.T) {
	account := &SettingsAccount{TypeName: "TELEGRAM", Settings: map[string]any{}}
	err := validateSettingsRequired(account)
	if err == nil || !strings.Contains(err.Error(), "Bot Token") {
		t.Fatalf("validate = %v, want missing Bot Token", err)
	}
	ok := &SettingsAccount{TypeName: "TELEGRAM", Settings: map[string]any{"botToken": "x"}}
	if err = validateSettingsRequired(ok); err != nil {
		t.Fatalf("validate with token should pass: %v", err)
	}
}
