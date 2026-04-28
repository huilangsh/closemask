package pii

import (
	"testing"
)

func TestShouldMask(t *testing.T) {
	tests := []struct {
		name      string
		typeName  string
		level     MaskLevel
		expected  bool
	}{
		// Strict 级别
		{"strict_phone", "PHONE", MaskLevelStrict, true},
		{"strict_email", "EMAIL", MaskLevelStrict, true},
		{"strict_idcard", "ID_CARD", MaskLevelStrict, true},
		{"strict_bankcard", "BANK_CARD", MaskLevelStrict, true},
		{"strict_ip", "IP_ADDRESS", MaskLevelStrict, true},
		{"strict_url", "URL_ADDRESS", MaskLevelStrict, true},
		{"strict_password", "PASSWORD", MaskLevelStrict, false}, // 0.60 < 0.8

		// Balanced 级别（默认）
		{"balanced_phone", "PHONE", MaskLevelBalanced, true},
		{"balanced_email", "EMAIL", MaskLevelBalanced, true},
		{"balanced_url", "URL_ADDRESS", MaskLevelBalanced, true},
		{"balanced_privatekey", "PRIVATE_KEY", MaskLevelBalanced, true},

		// Aggressive 级别
		{"aggressive_phone", "PHONE", MaskLevelAggressive, true},
		{"aggressive_url", "URL_ADDRESS", MaskLevelAggressive, true},
		{"aggressive_verification", "VERIFICATION_CODE", MaskLevelAggressive, true},
		{"aggressive_randomseed", "RANDOM_SEED", MaskLevelAggressive, true},

		// 未知类型
		{"unknown", "UNKNOWN_TYPE", MaskLevelBalanced, false}, // 0.5 < 0.6
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldMask(tt.typeName, tt.level)
			if result != tt.expected {
				t.Errorf("ShouldMask(%s, %s) = %v, want %v", tt.typeName, tt.level, result, tt.expected)
			}
		})
	}
}

func TestGetThreshold(t *testing.T) {
	tests := []struct {
		level    MaskLevel
		expected float64
	}{
		{MaskLevelStrict, 0.8},
		{MaskLevelBalanced, 0.6},
		{MaskLevelAggressive, 0.4},
		{MaskLevel("unknown"), 0.6}, // 默认 balanced
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			result := GetThreshold(tt.level)
			if result != tt.expected {
				t.Errorf("GetThreshold(%s) = %f, want %f", tt.level, result, tt.expected)
			}
		})
	}
}

func TestGetTypeConfidence(t *testing.T) {
	tests := []struct {
		typeName string
		expected float64
	}{
		{"ID_CARD", 0.90},
		{"BANK_CARD", 0.80},
		{"PHONE", 0.90},
		{"EMAIL", 0.90},
		{"IP_ADDRESS", 0.80},
		{"URL_ADDRESS", 0.80},
		{"PRIVATE_KEY", 0.95},
		{"VERIFICATION_CODE", 0.80},
		{"PASSWORD", 0.60},
		{"RANDOM_SEED", 0.70},
		{"UNKNOWN_TYPE", 0.5}, // 默认值
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			result := GetTypeConfidence(tt.typeName)
			if result != tt.expected {
				t.Errorf("GetTypeConfidence(%s) = %f, want %f", tt.typeName, result, tt.expected)
			}
		})
	}
}

func TestMaskLevelString(t *testing.T) {
	tests := []struct {
		level    MaskLevel
		expected string
	}{
		{MaskLevelStrict, "strict"},
		{MaskLevelBalanced, "balanced"},
		{MaskLevelAggressive, "aggressive"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.level) != tt.expected {
				t.Errorf("MaskLevel string = %s, want %s", string(tt.level), tt.expected)
			}
		})
	}
}

func TestParseMaskLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected MaskLevel
	}{
		{"strict", MaskLevelStrict},
		{"balanced", MaskLevelBalanced},
		{"aggressive", MaskLevelAggressive},
		{"unknown", MaskLevelBalanced}, // 默认
		{"", MaskLevelBalanced},        // 默认
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseMaskLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseMaskLevel(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfidenceTypeMapping(t *testing.T) {
	// 验证所有 PII 类型都有置信度配置
	piiTypes := []string{
		"ID_CARD", "BANK_CARD", "PHONE", "EMAIL", "IP_ADDRESS",
		"URL_ADDRESS", "PRIVATE_KEY", "VERIFICATION_CODE", "PASSWORD", "RANDOM_SEED",
	}

	for _, typeName := range piiTypes {
		t.Run(typeName, func(t *testing.T) {
			// 获取置信度
			confidence := GetTypeConfidence(typeName)
			if confidence < 0 || confidence > 1 {
				t.Errorf("Invalid confidence for %s: %f", typeName, confidence)
			}
		})
	}
}

func TestBuiltInDetector_WithLevel(t *testing.T) {
	// Strict 模式
	strictDetector := NewBuiltInPIIDetectorWithLevel(MaskLevelStrict)
	if strictDetector.level != MaskLevelStrict {
		t.Error("Strict detector should have strict level")
	}

	// Balanced 模式
	balancedDetector := NewBuiltInPIIDetectorWithLevel(MaskLevelBalanced)
	if balancedDetector.level != MaskLevelBalanced {
		t.Error("Balanced detector should have balanced level")
	}

	// Aggressive 模式
	aggressiveDetector := NewBuiltInPIIDetectorWithLevel(MaskLevelAggressive)
	if aggressiveDetector.level != MaskLevelAggressive {
		t.Error("Aggressive detector should have aggressive level")
	}
}

func TestBuiltInDetector_SetLevel(t *testing.T) {
	detector := NewBuiltInPIIDetector()

	// 默认应该是 balanced
	if detector.level != MaskLevelBalanced {
		t.Error("Default level should be balanced")
	}

	// 修改为 strict
	detector.SetLevel(MaskLevelStrict)
	if detector.level != MaskLevelStrict {
		t.Error("Level should be changed to strict")
	}
}
