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
