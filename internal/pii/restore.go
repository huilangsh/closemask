package pii

import (
	"regexp"
	"strings"
)

// placeholderSuffix 占位符后缀
const placeholderSuffix = "}"

// minPlaceholderLen 占位符最小长度 (${CRED_a1b2c3} = 15, 最短 ${X_0} = 6, 取合理值10)
const minPlaceholderLen = 10

// unrecoverablePlaceholder 还原失败时的替换文本
const unrecoverablePlaceholder = "[PII-UNRECOVERABLE]"

// ============ 新格式 fuzzy 正则 ============
// 匹配 LLM 可能输出的各种 ${TYPE_hash} 占位符变体：
// 标准格式: ${PHONE_a1b2c3}
// LLM 剥掉 ${}: PHONE_a1b2c3
// LLM 改为其他括号: [PHONE_a1b2c3], {PHONE_a1b2c3}, (PHONE_a1b2c3), <PHONE_a1b2c3>
// LLM 保留部分: PHONE_a1b2c3}, ${PHONE_a1b2c3
//
// 捕获组1: TYPE（大写字母+下划线）
// 捕获组2: hash（6-8位hex）
var fuzzyNewPlaceholderRe = regexp.MustCompile(`(?:\$\{|[<{\[(])([A-Z][A-Z_]{1,})_([a-f0-9]{6,8})(?:\}|[>}\])])?`)

// ============ 旧格式 fuzzy 正则（向后兼容 ${CRED_N}）============
// 匹配 LLM 可能输出的各种 ${CRED_N} 占位符变体
var fuzzyLegacyPlaceholderRe = regexp.MustCompile(`(?:\$\{|[<{\[(])?CRED[_ ]?(\d+)(?:\}|[>}\])])?`)

// RestoreArgs 递归还原参数中的占位符
func (h *PIIHandler) RestoreArgs(args interface{}, restoreFunc func(string) (string, bool)) interface{} {
	switch v := args.(type) {
	case string:
		// 先标准化再还原
		v = NormalizePlaceholders(v)
		// 先尝试整串匹配
		if val, ok := restoreFunc(v); ok {
			return val
		}
		// 整串未匹配，尝试子串扫描还原（处理占位符嵌入在长字符串中的情况）
		if ContainsPlaceholder(v) {
			return RestoreAll(v, restoreFunc)
		}
		return v

	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = h.RestoreArgs(val, restoreFunc)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = h.RestoreArgs(val, restoreFunc)
		}
		return result

	case float64: // JSON 数字解码为 float64
		return v

	case bool:
		return v

	case nil:
		return nil

	default:
		return v
	}
}

// RestoreAll 还原文本中的所有占位符
// 支持新格式 ${TYPE_hash} 和旧格式 ${CRED_N}
// 使用 strings.Builder 收集所有替换后一次性构建，避免多次字符串拼接
func RestoreAll(text string, restoreFunc func(string) (string, bool)) string {
	// 先标准化：修复 LLM 破坏的占位符格式
	text = NormalizePlaceholders(text)

	if !ContainsPlaceholder(text) {
		return text
	}

	var b strings.Builder
	b.Grow(len(text) + len(text)/4)
	lastEnd := 0
	start := 0

	for {
		// 找 ${
		startIdx := strings.Index(text[start:], "${")
		if startIdx == -1 {
			break
		}
		startIdx += start

		// 找最近的 }
		endIdx := strings.Index(text[startIdx+2:], "}")
		if endIdx == -1 {
			break
		}
		endIdx = startIdx + 2 + endIdx + 1 // +1 包含 }

		placeholder := text[startIdx:endIdx]
		if len(placeholder) < minPlaceholderLen {
			start = endIdx
			continue
		}

		// 验证是否是有效占位符（新格式或旧格式）
		isValid := IsPlaceholderToken(placeholder)

		if !isValid {
			// 不是有效占位符，跳过
			start = startIdx + 2 // 跳过 ${，继续搜索
			continue
		}

		// 写入占位符之前的文本
		b.WriteString(text[lastEnd:startIdx])

		if val, ok := restoreFunc(placeholder); ok {
			b.WriteString(val)
		} else {
			b.WriteString(unrecoverablePlaceholder)
		}

		lastEnd = endIdx
		start = endIdx
	}

	// 写入剩余文本
	if lastEnd < len(text) {
		b.WriteString(text[lastEnd:])
	}

	result := b.String()
	if result == "" {
		return text
	}
	return result
}

// IsPlaceholder 判断是否是占位符
// 支持新格式 ${TYPE_hash} 和旧格式 ${CRED_N}
func IsPlaceholder(s string) bool {
	return IsPlaceholderToken(s)
}

// NormalizePlaceholders 将 LLM 输出中被破坏的占位符格式修复为标准格式
// 支持新格式和旧格式的 fuzzy 修复
// 已是标准格式的不会被修改
func NormalizePlaceholders(text string) string {
	if !strings.Contains(text, "${") && !strings.Contains(text, "CRED") {
		return text
	}

	result := text

	// 1. 先修复新格式的 fuzzy 变体
	result = fuzzyNewPlaceholderRe.ReplaceAllStringFunc(result, func(match string) string {
		submatch := fuzzyNewPlaceholderRe.FindStringSubmatch(match)
		if len(submatch) < 3 {
			return match
		}
		piType := submatch[1]
		hash := submatch[2]
		// 如果已经是标准格式 ${TYPE_hash}，不修改
		standard := "${" + piType + "_" + hash + "}"
		if match == standard {
			return match
		}
		// 修复为标准格式
		return standard
	})

	// 2. 再修复旧格式的 fuzzy 变体（向后兼容）
	result = fuzzyLegacyPlaceholderRe.ReplaceAllStringFunc(result, func(match string) string {
		submatch := fuzzyLegacyPlaceholderRe.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		num := submatch[1]
		standard := "${CRED_" + num + "}"
		if match == standard {
			return match
		}
		return standard
	})

	return result
}
