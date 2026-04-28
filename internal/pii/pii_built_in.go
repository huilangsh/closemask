package pii

import (
	"encoding/json"
	"regexp"
)

// BuiltInPIIDetector 内置 PII 检测器
// 提供不依赖外部服务的 PII 检测能力，覆盖手机号、身份证、邮箱、银行卡等常见 PII
type BuiltInPIIDetector struct {
	patterns []piiPattern
	level    MaskLevel // 置信度级别
}

// piiPattern 单个 PII 检测模式
type piiPattern struct {
	regex   *regexp.Regexp
	typeName string // PII 类型名（如 PHONE, ID_CARD, EMAIL）
}

// piiEntity PII 实体（与 OneAIFW maskMeta 格式兼容）
type piiEntity struct {
	Type        string `json:"type"`
	Value       string `json:"value"`
	Placeholder string `json:"placeholder"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
}

// maskMetaContainer maskMeta JSON 容器
type maskMetaContainer struct {
	PII []piiEntity `json:"pii"`
}

// NewBuiltInPIIDetector 创建内置 PII 检测器
func NewBuiltInPIIDetector() *BuiltInPIIDetector {
	return NewBuiltInPIIDetectorWithLevel(MaskLevelBalanced)
}

// NewBuiltInPIIDetectorWithLevel 创建内置 PII 检测器（指定置信度级别）
func NewBuiltInPIIDetectorWithLevel(level MaskLevel) *BuiltInPIIDetector {
	d := &BuiltInPIIDetector{
		level: level,
	}
	d.initPatterns()
	return d
}

// SetLevel 设置置信度级别
func (d *BuiltInPIIDetector) SetLevel(level MaskLevel) {
	d.level = level
}

func (d *BuiltInPIIDetector) initPatterns() {
	// PII 模式定义：按优先级排序（长的模式优先匹配，避免短模式截断长模式）
	d.patterns = []piiPattern{
		// ========== 原有类型（5种） ==========

		// 身份证号（18位，含校验位）
		{
			typeName: "ID_CARD",
			regex:    regexp.MustCompile(`[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`),
		},
		// 银行卡号（16-19位纯数字，前后非数字）
		{
			typeName: "BANK_CARD",
			regex:    regexp.MustCompile(`\b\d{16,19}\b`),
		},
		// 中国手机号
		{
			typeName: "PHONE",
			regex:    regexp.MustCompile(`1[3-9]\d{9}`),
		},
		// 邮箱地址
		{
			typeName: "EMAIL",
			regex:    regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		},
		// IPv4 地址
		{
			typeName: "IP_ADDRESS",
			regex:    regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		},

		// ========== 新增类型（5种） ==========

		// URL 地址
		{
			typeName: "URL_ADDRESS",
			regex:    regexp.MustCompile(`https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%\-]+`),
		},
		// PEM 格式私钥（高置信度）
		{
			typeName: "PRIVATE_KEY",
			regex:    regexp.MustCompile(`-----BEGIN (?:OPENSSH|RSA|EC|DSA|PRIVATE) KEY-----[\s\S]*?-----END (?:OPENSSH|RSA|EC|DSA|PRIVATE) KEY-----`),
		},
		// 带标签的验证码（支持"验证码是"、"验证码为"、"verification code is"等格式）
		{
			typeName: "VERIFICATION_CODE",
			regex:    regexp.MustCompile(`(?i)(?:验证码|verification\s*code|verify\s*code|otp|2fa\s*code|auth(?:entication)?\s*code)\s*(?:是|为|[:：=\-]|is\s+)\s*([A-Za-z0-9]{4,12})`),
		},
		// 密码（支持"密码是"、"密码为"、"password is"、"pwd is"等格式）
		// 注意：必须有关键词后的分隔符（是/为/:/=/is），避免误匹配"密码错误"等
		// 密码值只匹配非空白、非中文的字符序列（通常为字母数字和特殊字符）
		{
			typeName: "PASSWORD",
			regex:    regexp.MustCompile(`(?i)(?:password|密码|pwd|pass|passwd)\s*(?:是|为|[:：=]|is\s+)\s*([a-zA-Z0-9!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]+)`),
		},
		// 助记词（加密货币钱包，支持"助记词是"、"助记词为"、"seed is"等格式）
		{
			typeName: "RANDOM_SEED",
			regex:    regexp.MustCompile(`(?i)(?:seed|mnemonic|助记词)\s*(?:是|为|[:：=]|is\s+)\s*((?:[a-z]+\s+){11,23}[a-z]+)`),
		},

		// ========== 国际格式（3种） ==========

		// 国际手机号（E.164 格式，如 +1-555-123-4567）
		{
			typeName: "PHONE_INTERNATIONAL",
			regex:    regexp.MustCompile(`\+(?:[0-9][\-\s]?){6,14}[0-9]`),
		},
		// 美国社会安全号 SSN（如 123-45-6789）
		{
			typeName: "SSN",
			regex:    regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		},
		// 英国国家保险号 NINO（如 AB123456C）
		{
			typeName: "NINO",
			regex:    regexp.MustCompile(`\b[A-CEGHJ-PR-TW-Z][A-CEGHJ-NPR-TW-Z]\d{6}[A-D]\b`),
		},

		// ========== 扩展类型（8种） ==========

		// 信用卡号（13-19位，需 Luhn 校验）
		{
			typeName: "CREDIT_CARD",
			regex:    regexp.MustCompile(`\b(?:\d{4}[\s\-]?){3}\d{1,4}\b|\b\d{13,19}\b`),
		},
		// 中国车牌号（含新能源车牌）
		{
			typeName: "LICENSE_PLATE",
			regex:    regexp.MustCompile(`[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼使领][A-HJ-NP-Z][A-HJ-NP-Z0-9]{4,5}[A-HJ-NP-Z0-9挂学警港澳]`),
		},
		// 统一社会信用代码（18位）
		{
			typeName: "CREDIT_CODE",
			regex:    regexp.MustCompile(`[0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10}`),
		},
		// 中国护照号码（如 P12345678, E12345678, G12345678）
		{
			typeName: "PASSPORT",
			regex:    regexp.MustCompile(`\b[PEG]\d{8}\b`),
		},
		// IPv6 地址
		{
			typeName: "IPV6_ADDRESS",
			regex:    regexp.MustCompile(`(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|(?:[0-9a-fA-F]{1,4}:){1,7}:|(?:[0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|::(?:[0-9a-fA-F]{1,4}:){0,5}[0-9a-fA-F]{1,4}`),
		},
		// JWT Token
		{
			typeName: "JWT_TOKEN",
			regex:    regexp.MustCompile(`\beyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\b`),
		},
		// AWS Access Key ID
		{
			typeName: "AWS_KEY_ID",
			regex:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		},
		// 敏感文件路径
		{
			typeName: "SENSITIVE_PATH",
			regex:    regexp.MustCompile(`(?:/\.ssh/|/\.pgp/|/\.gnupg/|\.pem$|\.key$|\.priv$|id_rsa|id_dsa|id_ecdsa|id_ed25519)`),
		},
	}
}

// DetectAndMask 检测文本中的 PII 并遮罩，返回遮罩后的文本和 maskMeta JSON
// maskMeta 与 OneAIFW 格式完全兼容，可被 extractPlaceholders 直接解析
func (d *BuiltInPIIDetector) DetectAndMask(text string, addPlaceholder func(placeholder, value string)) (string, string) {
	if text == "" {
		return text, `{"pii":[]}`
	}

	entities := make([]piiEntity, 0)
	result := text

	// 按优先级依次匹配每种 PII 类型
	for _, p := range d.patterns {
		// 置信度过滤
		if !ShouldMask(p.typeName, d.level) {
			continue
		}

		// 从后往前替换，避免索引偏移
		matches := p.regex.FindAllStringSubmatchIndex(result, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]

			// 确定值的范围
			// 如果有捕获组（m[2], m[3]），使用捕获组的范围
			// 否则使用整个匹配的范围（m[0], m[1]）
			var valueStart, valueEnd int
			var originalValue string

			if len(m) >= 4 && m[2] >= 0 {
				// 有捕获组，使用捕获组的值
				valueStart = m[2]
				valueEnd = m[3]
				originalValue = result[valueStart:valueEnd]
			} else {
				// 无捕获组，使用整个匹配
				valueStart = m[0]
				valueEnd = m[1]
				originalValue = result[valueStart:valueEnd]
			}

			// 跳过已经是占位符的
			if IsPlaceholder(originalValue) {
				continue
			}
			// 跳过在占位符内部的
			if isInsidePlaceholder(result, valueStart) {
				continue
			}

			placeholder := GeneratePlaceholder(p.typeName, originalValue)

			// 记录实体
			entities = append(entities, piiEntity{
				Type:        p.typeName,
				Value:       originalValue,
				Placeholder: placeholder,
				Start:       valueStart,
				End:         valueEnd,
			})

			// 通知调用方（保存到 session）
			if addPlaceholder != nil {
				addPlaceholder(placeholder, originalValue)
			}

			// 替换文本
			result = result[:valueStart] + placeholder + result[valueEnd:]
		}
	}

	// 构建 maskMeta JSON（与 OneAIFW 格式兼容）
	meta := maskMetaContainer{PII: entities}
	metaJSON, _ := json.Marshal(meta)

	return result, string(metaJSON)
}
