package pii

import (
	"strings"
	"testing"
)

func TestLocalMasker_Off(t *testing.T) {
	lm := NewLocalMasker("off")
	result := lm.Mask("api_key=sk-test1234567890", func(_, _ string) {})
	if result != "api_key=sk-test1234567890" {
		t.Errorf("off mode should not mask, got: %s", result)
	}
}

func TestLocalMasker_KeyNameMatch(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	result := lm.Mask("OPENAI_API_KEY=sk-proj-abc123def456789", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "sk-proj-abc123def456789") {
		t.Errorf("API key value should be masked, got: %s", result)
	}
	if len(placeholders) == 0 {
		t.Error("Should have at least one placeholder")
	}

	// Check value is preserved
	for _, v := range placeholders {
		if v != "sk-proj-abc123def456789" {
			t.Errorf("Placeholder value should be original, got: %s", v)
		}
	}

	// Check key name is preserved
	if !strings.Contains(result, "OPENAI_API_KEY=") {
		t.Errorf("Key name should be preserved, got: %s", result)
	}
}

func TestLocalMasker_KeyNameMatch_DBURL(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	result := lm.Mask("DATABASE_URL=postgres://admin:secret123@db.example.com:5432/mydb", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "secret123") {
		t.Errorf("DB password should be masked, got: %s", result)
	}
	if !strings.Contains(result, "DATABASE_URL=") {
		t.Errorf("Key name should be preserved, got: %s", result)
	}
}

func TestLocalMasker_KeyNameMatch_Colon(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	result := lm.Mask("api_key: mysecretkey12345", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "mysecretkey12345") {
		t.Errorf("api_key value should be masked, got: %s", result)
	}
	if !strings.Contains(result, "api_key:") {
		t.Errorf("Key name should be preserved, got: %s", result)
	}
}

func TestLocalMasker_KeyNameMatch_ChineseLLM(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"DASHSCOPE", "DASHSCOPE_API_KEY=dashscope-abc123def456"},
		{"ZHIPU", "ZHIPU_API_KEY=zhipu-abc123def456"},
		{"DEEPSEEK", "DEEPSEEK_API_KEY=deepseek-abc123def456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lm := NewLocalMasker("strict")
			placeholders := make(map[string]string)
			result := lm.Mask(tt.input, func(p, v string) {
				placeholders[p] = v
			})
			if len(placeholders) == 0 {
				t.Errorf("Should mask %s, got: %s", tt.name, result)
			}
		})
	}
}

func TestLocalMasker_ValueFormat_BearerJWT(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	result := lm.Mask("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "eyJhbGci") {
		t.Errorf("JWT should be masked, got: %s", result)
	}
}

func TestLocalMasker_ValueFormat_AWSAK(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	result := lm.Mask("AWS key: AKIAIOSFODNN7EXAMPLE", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS AK should be masked, got: %s", result)
	}
}

func TestLocalMasker_NoMaskOnShortValue(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	result := lm.Mask("api_key=short", func(p, v string) {
		placeholders[p] = v
	})

	if len(placeholders) > 0 {
		t.Errorf("Short values (<8 chars) should not be masked, got: %s, placeholders: %v", result, placeholders)
	}
}

func TestLocalMasker_PlaceholderFormat(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	lm.Mask("api_key=secretvalue12345", func(p, v string) {
		placeholders[p] = v
	})

	for p := range placeholders {
		if !strings.HasPrefix(p, "${CRED_") || !strings.HasSuffix(p, "}") {
			t.Errorf("Placeholder format should be ${CRED_N}, got: %s", p)
		}
	}
}

// ===== RestoreAll tests =====

func TestRestoreAll_Unrecoverable(t *testing.T) {
	text := "Hello ${PHONE_a1b2c3}, your key is ${CRED_d4e5f6}"
	restoreFunc := func(placeholder string) (string, bool) {
		if placeholder == "${PHONE_a1b2c3}" {
			return "13800138000", true
		}
		return "", false
	}

	result := RestoreAll(text, restoreFunc)
	if !strings.Contains(result, "13800138000") {
		t.Errorf("Known placeholder should be restored, got: %s", result)
	}
	if !strings.Contains(result, "[PII-UNRECOVERABLE]") {
		t.Errorf("Unknown placeholder should be [PII-UNRECOVERABLE], got: %s", result)
	}
}

// ===== RestoreArgs substring restore tests =====

func TestRestoreArgs_SubstringRestore(t *testing.T) {
	handler := &PIIHandler{}
	args := map[string]interface{}{
		"url": "postgres://admin:secret123@db.example.com:5432/mydb",
	}

	restoreFunc := func(placeholder string) (string, bool) {
		if placeholder == "${CRED_a1b2c3}" {
			return "secret123", true
		}
		return "", false
	}

	// First, manually replace to simulate masking
	args["url"] = "postgres://admin:${CRED_a1b2c3}@db.example.com:5432/mydb"

	result := handler.RestoreArgs(args, restoreFunc)
	resultMap := result.(map[string]interface{})
	url := resultMap["url"].(string)

	if !strings.Contains(url, "secret123") {
		t.Errorf("Embedded placeholder should be restored, got: %s", url)
	}
	if strings.Contains(url, "${CRED_a1b2c3}") {
		t.Errorf("Placeholder should be replaced, got: %s", url)
	}
}

func TestRestoreArgs_FullStringMatch(t *testing.T) {
	handler := &PIIHandler{}
	args := map[string]interface{}{
		"key": "${CRED_a1b2c3}",
	}

	restoreFunc := func(placeholder string) (string, bool) {
		if placeholder == "${CRED_a1b2c3}" {
			return "sk-proj-abc123", true
		}
		return "", false
	}

	result := handler.RestoreArgs(args, restoreFunc)
	resultMap := result.(map[string]interface{})
	key := resultMap["key"].(string)

	if key != "sk-proj-abc123" {
		t.Errorf("Full string match should work, got: %s", key)
	}
}

// ===== Aggressive 值格式匹配测试 =====

func TestLocalMasker_Aggressive_PkUUID(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask("my key is pk-1148a252-026e-4aa3-a69f-ab16eef56a3e here", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "pk-1148a252-026e-4aa3-a69f-ab16eef56a3e") {
		t.Errorf("pk-UUID should be masked, got: %s", result)
	}
	if len(placeholders) == 0 {
		t.Error("Should have at least one placeholder for pk-UUID")
	}
	for _, v := range placeholders {
		if v == "pk-1148a252-026e-4aa3-a69f-ab16eef56a3e" {
			return // found
		}
	}
	t.Error("pk-UUID value should be preserved in placeholder map")
}

func TestLocalMasker_Aggressive_SkPrefix(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask("key: sk-proj-abc123def4567890abcdefghij", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "sk-proj-abc123def4567890abcdefghij") {
		t.Errorf("sk- prefix key should be masked, got: %s", result)
	}
	if len(placeholders) == 0 {
		t.Error("Should have at least one placeholder for sk- key")
	}
}

func TestLocalMasker_Aggressive_UUIDOnly(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask("uuid: 1148a252-026e-4aa3-a69f-ab16eef56a3e", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "1148a252-026e-4aa3-a69f-ab16eef56a3e") {
		t.Errorf("Standalone UUID should be masked, got: %s", result)
	}
	if len(placeholders) == 0 {
		t.Error("Should have at least one placeholder for standalone UUID")
	}
}

func TestLocalMasker_Aggressive_JSON_ApiKey(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	input := `"apiKey": "pk-1148a252-026e-4aa3-a69f-ab16eef56a3e"`
	result := lm.Mask(input, func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "pk-1148a252-026e-4aa3-a69f-ab16eef56a3e") {
		t.Errorf("JSON apiKey value should be masked, got: %s", result)
	}
	if !strings.Contains(result, `"apiKey":`) {
		t.Errorf("Key name should be preserved, got: %s", result)
	}
}

func TestLocalMasker_Strict_NoAggressiveValMatch(t *testing.T) {
	lm := NewLocalMasker("strict")
	placeholders := make(map[string]string)
	// 独立的 pk-UUID 在 strict 模式下不应被遮罩（没有键名关联）
	result := lm.Mask("random text pk-1148a252-026e-4aa3-a69f-ab16eef56a3e here", func(p, v string) {
		placeholders[p] = v
	})

	if strings.Contains(result, "${CRED_") {
		t.Errorf("Strict mode should not mask pk-UUID without key name, got: %s", result)
	}
}

// ===== 英文场景综合测试 =====

func TestEnglish_PhoneInSentence(t *testing.T) {
	detector := NewBuiltInPIIDetector()
	placeholders := make(map[string]string)
	result, _ := detector.DetectAndMask("My phone number is 13912345678, please call me.", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "13912345678") {
		t.Errorf("Phone should be masked, got: %s", result)
	}
	if len(placeholders) == 0 {
		t.Error("Phone should be detected")
	}
}

func TestEnglish_EmailInSentence(t *testing.T) {
	detector := NewBuiltInPIIDetector()
	placeholders := make(map[string]string)
	result, _ := detector.DetectAndMask("Please send the report to john.doe@example.com before Friday.", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "john.doe@example.com") {
		t.Errorf("Email should be masked, got: %s", result)
	}
}

func TestEnglish_IDCardNoPhoneFalsePositive(t *testing.T) {
	detector := NewBuiltInPIIDetector()
	placeholders := make(map[string]string)
	result, _ := detector.DetectAndMask("The customer ID is 110101199003076534, please verify.", func(p, v string) {
		placeholders[p] = v
	})
	// 身份证号应该被遮罩
	if strings.Contains(result, "110101199003076534") {
		t.Errorf("ID card should be masked, got: %s", result)
	}
	// 不应该有 PHONE 占位符（身份证号中的子串不应被误检为手机号）
	for k, v := range placeholders {
		if strings.HasPrefix(k, "${PHONE_") {
			t.Errorf("ID card should not produce PHONE placeholder, got: %s -> %s", k, v)
		}
	}
}

func TestEnglish_MultiPII(t *testing.T) {
	detector := NewBuiltInPIIDetector()
	placeholders := make(map[string]string)
	result, _ := detector.DetectAndMask("Contact John at john@example.com or call 13912345678.", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "john@example.com") {
		t.Errorf("Email should be masked, got: %s", result)
	}
	if strings.Contains(result, "13912345678") {
		t.Errorf("Phone should be masked, got: %s", result)
	}
}

func TestEnglish_Aggressive_ApiKey(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask(`{"apiKey": "pk-1148a252-026e-4aa3-a69f-ab16eef56a3e", "model": "gpt-4"}`, func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "pk-1148a252-026e-4aa3-a69f-ab16eef56a3e") {
		t.Errorf("API key should be masked, got: %s", result)
	}
}

func TestEnglish_Aggressive_SkPrefix(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask("I accidentally committed sk-proj-abc123def4567890abcdefghij to the repo!", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "sk-proj-abc123def4567890abcdefghij") {
		t.Errorf("sk- key should be masked, got: %s", result)
	}
}

func TestEnglish_Aggressive_BearerJWT(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "eyJhbGci") {
		t.Errorf("JWT should be masked, got: %s", result)
	}
}

func TestEnglish_Aggressive_AWSKey(t *testing.T) {
	lm := NewLocalMasker("aggressive")
	placeholders := make(map[string]string)
	result := lm.Mask("My AWS access key is AKIAIOSFODNN7EXAMPLE for production.", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS AK should be masked, got: %s", result)
	}
}

func TestEnglish_IPAddress(t *testing.T) {
	detector := NewBuiltInPIIDetector()
	placeholders := make(map[string]string)
	result, _ := detector.DetectAndMask("Server IP is 192.168.1.100, internal only.", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "192.168.1.100") {
		t.Errorf("IP address should be masked, got: %s", result)
	}
}

func TestEnglish_BankCard(t *testing.T) {
	detector := NewBuiltInPIIDetector()
	placeholders := make(map[string]string)
	result, _ := detector.DetectAndMask("Card number 6222021234567890123, please charge.", func(p, v string) {
		placeholders[p] = v
	})
	if strings.Contains(result, "6222021234567890123") {
		t.Errorf("Bank card should be masked, got: %s", result)
	}
}
