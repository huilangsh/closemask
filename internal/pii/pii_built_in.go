package pii

import (
	"encoding/json"
	"regexp"
)

// BuiltInPIIDetector 内置 PII 检测器
// 提供不依赖外部服务的 PII 检测能力，覆盖手机号、身份证、邮箱、银行卡等常见 PII
type BuiltInPIIDetector struct {
	patterns []piiPattern
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
	d := &BuiltInPIIDetector{}
	d.initPatterns()
	return d
}

func (d *BuiltInPIIDetector) initPatterns() {
	// PII 模式定义：按优先级排序（长的模式优先匹配，避免短模式截断长模式）
	d.patterns = []piiPattern{
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
		// 从后往前替换，避免索引偏移
		matches := p.regex.FindAllStringSubmatchIndex(result, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			valueStart := m[0]
			valueEnd := m[1]
			originalValue := result[valueStart:valueEnd]

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
