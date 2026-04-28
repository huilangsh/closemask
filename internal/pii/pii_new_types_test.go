package pii

import (
	"encoding/json"
	"testing"
)

func TestNewPIITypes(t *testing.T) {
	detector := NewBuiltInPIIDetector()

	tests := []struct {
		name     string
		text     string
		expected []string // 期望检测到的 PII 类型
	}{
		// ========== 原有类型 ==========
		{
			name:     "中国身份证",
			text:     "张三的身份证号是110101199001011234",
			expected: []string{"ID_CARD"},
		},
		{
			name:     "银行卡号",
			text:     "银行卡号6225880212345678",
			expected: []string{"BANK_CARD"},
		},
		{
			name:     "中国手机号",
			text:     "联系电话13812345678",
			expected: []string{"PHONE"},
		},
		{
			name:     "邮箱地址",
			text:     "邮箱test@example.com",
			expected: []string{"EMAIL"},
		},
		{
			name:     "IPv4地址",
			text:     "服务器IP是192.168.1.100",
			expected: []string{"IP_ADDRESS"},
		},
		{
			name:     "URL地址",
			text:     "访问https://example.com/path",
			expected: []string{"URL_ADDRESS"},
		},
		{
			name:     "验证码-中文",
			text:     "验证码是AB12CD",
			expected: []string{"VERIFICATION_CODE"},
		},
		{
			name:     "验证码-英文",
			text:     "verification code is CODE123",
			expected: []string{"VERIFICATION_CODE"},
		},
		{
			name:     "密码-中文",
			text:     "密码是Admin@123",
			expected: []string{"PASSWORD"},
		},
		{
			name:     "密码-英文",
			text:     "password is secret123",
			expected: []string{"PASSWORD"},
		},
		{
			name:     "助记词",
			text:     "助记词是abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
			expected: []string{"RANDOM_SEED"},
		},

		// ========== 国际格式 ==========
		{
			name:     "国际手机号",
			text:     "国际号码+1-555-123-4567",
			expected: []string{"PHONE_INTERNATIONAL"},
		},
		{
			name:     "美国SSN",
			text:     "SSN是123-45-6789",
			expected: []string{"SSN"},
		},
		{
			name:     "英国NINO",
			text:     "NINO是AB123456C",
			expected: []string{"NINO"},
		},

		// ========== 扩展类型 ==========
		{
			name:     "信用卡号-带分隔符",
			text:     "信用卡号4532-1234-5678-9010",
			expected: []string{"CREDIT_CARD"},
		},
		{
			name:     "信用卡号-纯数字",
			text:     "信用卡号4532123456789010",
			expected: []string{"CREDIT_CARD"},
		},
		{
			name:     "中国车牌",
			text:     "车牌号京A12345",
			expected: []string{"LICENSE_PLATE"},
		},
		{
			name:     "新能源车牌",
			text:     "新能源车牌京AD12345",
			expected: []string{"LICENSE_PLATE"},
		},
		{
			name:     "统一社会信用代码",
			text:     "信用代码91110000600007336F",
			expected: []string{"CREDIT_CODE"},
		},
		{
			name:     "中国护照",
			text:     "护照号P12345678",
			expected: []string{"PASSPORT"},
		},
		{
			name:     "IPv6地址",
			text:     "IPv6地址2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expected: []string{"IPV6_ADDRESS"},
		},
		{
			name:     "JWT Token",
			text:     "JWT是eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: []string{"JWT_TOKEN"},
		},
		{
			name:     "AWS Key ID",
			text:     "AWS Key是AKIAIOSFODNN7EXAMPLE",
			expected: []string{"AWS_KEY_ID"},
		},
		{
			name:     "敏感路径-ssh",
			text:     "私钥路径/home/user/.ssh/id_rsa",
			expected: []string{"SENSITIVE_PATH"},
		},
		{
			name:     "敏感路径-pem",
			text:     "证书路径/etc/ssl/cert.pem",
			expected: []string{"SENSITIVE_PATH"},
		},

		// ========== 混合测试 ==========
		{
			name:     "混合PII",
			text:     "用户张三，手机13812345678，邮箱test@example.com，身份证110101199001011234",
			expected: []string{"PHONE", "EMAIL", "ID_CARD"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masked, metaJSON := detector.DetectAndMask(tt.text, nil)

			// 解析 metaJSON 获取检测到的类型
			var meta struct {
				PII []struct {
					Type string `json:"type"`
				} `json:"pii"`
			}
			if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
				t.Fatalf("解析 metaJSON 失败: %v", err)
			}

			// 提取检测到的类型
			detected := make(map[string]bool)
			for _, e := range meta.PII {
				detected[e.Type] = true
			}

			// 检查期望的类型是否都被检测到
			for _, exp := range tt.expected {
				if !detected[exp] {
					t.Errorf("期望检测到 %s，但未检测到", exp)
				}
			}

			// 检查是否有遮罩
			if len(meta.PII) == 0 {
				t.Errorf("未检测到任何 PII")
			}

			t.Logf("原文: %s", tt.text)
			t.Logf("遮罩: %s", masked)
			t.Logf("检测: %v", meta.PII)
		})
	}
}
