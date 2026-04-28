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
// 注意：此正则仅用于修复非标准格式。标准格式 ${TYPE_hash} 不应被修改。
// 捕获组1: TYPE（大写字母+下划线）
// 捕获组2: hash（6-8位hex）
var fuzzyNewPlaceholderRe = regexp.MustCompile(`(?:\$\{|[<{\[(])([A-Z][A-Z_]{1,})_([a-f0-9]{6,8})(?:\}|[>}\])])?`)

// standardPlaceholderRe 匹配标准格式占位符 ${TYPE_hash}
// 用于快速检测，避免 fuzzy 正则误处理
var standardPlaceholderRe = regexp.MustCompile(`\$\{[A-Z][A-Z_]{1,}_[a-f0-9]{6,8}\}`)

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

	// 在函数内部编译正则表达式，确保正确初始化
	standardRe := regexp.MustCompile(`\$\{[A-Z][A-Z_]{1,}_[a-f0-9]{6,8}\}`)
	
	// 先提取所有标准格式占位符，保护它们不被 fuzzy 正则误处理
	standardPlaceholders := standardRe.FindAllString(text, -1)
	standardMap := make(map[string]bool)
	for _, p := range standardPlaceholders {
		standardMap[p] = true
	}

	result := text

	// 1. 先修复新格式的 fuzzy 变体（跳过已是标准格式的）
	result = fuzzyNewPlaceholderRe.ReplaceAllStringFunc(result, func(match string) string {
		// 如果已经是标准格式，直接返回
		if standardMap[match] {
			return match
		}
		submatch := fuzzyNewPlaceholderRe.FindStringSubmatch(match)
		if len(submatch) < 3 {
			return match
		}
		piType := submatch[1]
		hash := submatch[2]
		// 修复为标准格式
		standard := "${" + piType + "_" + hash + "}"
		return standard
	})

	// 2. 再修复旧格式的 fuzzy 变体（向后兼容）
	// 注意：fuzzyLegacyPlaceholderRe 可能错误匹配新格式的一部分（如 ${CRED_5）
	// 需要检查匹配结果是否是标准格式的一部分
	result = fuzzyLegacyPlaceholderRe.ReplaceAllStringFunc(result, func(match string) string {
		// 检查匹配结果后面是否紧跟着 hex 字符（说明是新格式的一部分）
		// 如果是，跳过
		submatch := fuzzyLegacyPlaceholderRe.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		num := submatch[1]
		standard := "${CRED_" + num + "}"
		if match == standard {
			return match
		}
		// 检查：如果 match 后面紧跟着 hex 字符（a-f0-9），说明是新格式的一部分，跳过
		// 例如：${CRED_5 后面是 c2a9c}，说明是 ${CRED_5c2a9c} 的一部分
		// 通过检查原始文本中 match 后面的字符来判断
		matchIdx := strings.Index(result, match)
		if matchIdx != -1 {
			afterMatch := ""
			if matchIdx+len(match) < len(result) {
				afterMatch = result[matchIdx+len(match):]
			}
			// 如果后面紧跟 hex 字符，跳过
			if len(afterMatch) > 0 {
				firstChar := afterMatch[0]
				if (firstChar >= 'a' && firstChar <= 'f') || (firstChar >= '0' && firstChar <= '9') {
					// 是新格式的一部分，跳过
					return match
				}
			}
		}
		return standard
	})

	return result
}
