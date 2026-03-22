package pii

import (
	"strings"
)

// RestoreArgs 递归还原参数中的占位符
func (h *PIIHandler) RestoreArgs(args interface{}, restoreFunc func(string) (string, bool)) interface{} {
	switch v := args.(type) {
	case string:
		if val, ok := restoreFunc(v); ok {
			return val
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

// RestoreAll 还原文本中的所有占位符（简化版）
func RestoreAll(text string, restoreFunc func(string) (string, bool)) string {
	result := text

	// 找到所有占位符格式：__TYPE_INDEX__ 或 __TYPE_TIMESTAMP_COUNTER__
	// 简单实现：遍历查找并替换
	start := 0
	for {
		startIdx := strings.Index(result[start:], "__")
		if startIdx == -1 {
			break
		}
		startIdx += start

		endIdx := strings.Index(result[startIdx:], "__")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + 2 // 包含后面的 "__"

		placeholder := result[startIdx:endIdx]
		if val, ok := restoreFunc(placeholder); ok {
			result = result[:startIdx] + val + result[endIdx:]
			// 替换后长度变化，不需要调整 start
		} else {
			start = endIdx
		}
	}

	return result
}

// IsPlaceholder 判断是否是占位符
func IsPlaceholder(s string) bool {
	return strings.HasPrefix(s, "__") && strings.HasSuffix(s, "__")
}
