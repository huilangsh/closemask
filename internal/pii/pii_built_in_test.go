package pii

import (
	"strings"
	"testing"
)

func TestPasswordRegex(t *testing.T) {
	detector := NewBuiltInPIIDetector()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// 中文格式
		{"密码是", "Wi-Fi密码是 Admin@123456", true},
		{"密码为", "密码为 abc123", true},
		{"密码：", "密码：secret123", true},
		{"密码=", "密码=pass123", true},

		// 英文格式
		{"password is", "password is secret123", true},
		{"password:", "password: mypass123", true},
		{"password=", "password=test123", true},
		{"pwd is", "pwd is abc", true},
		{"pwd:", "pwd: xyz", true},
		{"pass:", "pass: 123", true},
		{"passwd:", "passwd: secret", true},

		// 不应该匹配
		{"普通文本", "这个密码错误", false},
		{"无密码", "请输入密码", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var foundPassword bool
			addPlaceholder := func(placeholder, value string) {
				if strings.Contains(placeholder, "PASSWORD") {
					foundPassword = true
				}
			}

			result, _ := detector.DetectAndMask(tt.input, addPlaceholder)

			if foundPassword != tt.expected {
				t.Errorf("Test %s: expected foundPassword=%v, got %v", tt.name, tt.expected, foundPassword)
				t.Errorf("Input: %s", tt.input)
				t.Errorf("Result: %s", result)
			}
		})
	}
}

func TestVerificationCodeRegex(t *testing.T) {
	detector := NewBuiltInPIIDetector()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// 中文格式
		{"验证码是", "验证码是 123456", true},
		{"验证码为", "验证码为 ABC123", true},
		{"验证码：", "验证码：1234", true},
		{"验证码=", "验证码=567890", true},

		// 英文格式
		{"verification code is", "verification code is ABC123", true},
		{"verification code:", "verification code: 123456", true},
		{"verify code:", "verify code: ABCD", true},
		{"otp:", "otp: 123456", true},
		{"2fa code:", "2fa code: 123456", true},
		{"auth code:", "auth code: ABCD", true},
		{"authentication code:", "authentication code: 1234", true},

		// 不应该匹配
		{"普通文本", "请输入验证码", false},
		{"无验证码", "验证码错误", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var foundCode bool
			addPlaceholder := func(placeholder, value string) {
				if strings.Contains(placeholder, "VERIFICATION_CODE") {
					foundCode = true
				}
			}

			result, _ := detector.DetectAndMask(tt.input, addPlaceholder)

			if foundCode != tt.expected {
				t.Errorf("Test %s: expected foundCode=%v, got %v", tt.name, tt.expected, foundCode)
				t.Errorf("Input: %s", tt.input)
				t.Errorf("Result: %s", result)
			}
		})
	}
}

func TestRandomSeedRegex(t *testing.T) {
	detector := NewBuiltInPIIDetector()

	// 12个单词的助记词
	mnemonic12 := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	// 24个单词的助记词
	mnemonic24 := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art"

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// 中文格式
		{"助记词是", "助记词是 " + mnemonic12, true},
		{"助记词为", "助记词为 " + mnemonic24, true},
		{"助记词：", "助记词：" + mnemonic12, true},

		// 英文格式
		{"seed is", "seed is " + mnemonic12, true},
		{"seed:", "seed: " + mnemonic24, true},
		{"mnemonic:", "mnemonic: " + mnemonic12, true},

		// 不应该匹配（单词数量不足）
		{"单词不足", "助记词是 abandon abandon abandon", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var foundSeed bool
			addPlaceholder := func(placeholder, value string) {
				if strings.Contains(placeholder, "RANDOM_SEED") {
					foundSeed = true
				}
			}

			result, _ := detector.DetectAndMask(tt.input, addPlaceholder)

			if foundSeed != tt.expected {
				t.Errorf("Test %s: expected foundSeed=%v, got %v", tt.name, tt.expected, foundSeed)
				t.Errorf("Input: %s", tt.input)
				t.Errorf("Result: %s", result)
			}
		})
	}
}

func TestAuditDialogue(t *testing.T) {
	detector := NewBuiltInPIIDetector()

	dialogue := `用户：你好，我是王建国。我之前报修的工单进度怎么样了？我的手机号是 13912349876，邮箱是 wangjianguo@163.com。

客服：王先生您好，查到您的工单了。为了进一步核实身份，请提供一下您的身份证号和收货地址。

用户：身份证号是 110101199505061234。地址是上海市浦东新区陆家嘴环路1000号恒生银行大厦15层1502室。

客服：好的已记录。另外您之前提供的退款银行卡号 6225880134567890（招商银行），由于系统升级需要重新绑定。还有您的订单号 2023110598765 也在这个工单里。

用户：行，那我新卡号发你：6217001234567890123。对了，我妻子赵敏也想查一下她的信息，她的护照号是 E12345678。

客服：可以的。顺便提醒一下，您在系统里留的Wi-Fi密码是 Admin@123456，这个建议修改。如果有技术问题，也可以直接用我们的内网IP 10.0.0.55 联系运维部门。

用户：收到。另外那个充值卡的卡密是 ABCD-1234-EFGH-5678，你帮我核实下有没有到账。`

	placeholders := make(map[string]string)
	addPlaceholder := func(placeholder, value string) {
		placeholders[placeholder] = value
	}

	result, metaJSON := detector.DetectAndMask(dialogue, addPlaceholder)

	t.Logf("遮罩后的文本:\n%s", result)
	t.Logf("maskMeta: %s", metaJSON)
	t.Logf("找到 %d 个 PII", len(placeholders))

	// 检查密码是否被遮罩
	passwordFound := false
	for placeholder := range placeholders {
		if strings.Contains(placeholder, "PASSWORD") {
			passwordFound = true
			break
		}
	}

	if !passwordFound {
		t.Error("密码 'Admin@123456' 未被遮罩！")
	}

	// 检查手机号是否被遮罩
	phoneFound := false
	for placeholder := range placeholders {
		if strings.Contains(placeholder, "PHONE") {
			phoneFound = true
			break
		}
	}

	if !phoneFound {
		t.Error("手机号未被遮罩！")
	}
}
