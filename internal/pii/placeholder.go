package pii

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// PlaceholderGenerator 确定性占位符生成器
// 基于 PII 值的哈希生成占位符，同一值永远生成同一占位符
type PlaceholderGenerator struct {
	hashLength int    // 哈希截取长度（6 或 8）
	hmacKey    []byte // HMAC 密钥（空则用 plain sha256）
}

// 占位符格式: ${TYPE_hash}
// TYPE: 大写字母+下划线，如 CRED, PHONE, ID_CARD, EMAIL, BANK_CARD, IP_ADDRESS
// hash: sha256/hmac-sha256 的前 hashLength 位 hex

// placeholderRegex 匹配标准占位符格式 ${TYPE_hash}
// TYPE: 至少2的大写字母或下划线
// hash: 6或8位hex
var placeholderRegex = regexp.MustCompile(`^\$\{([A-Z][A-Z_]{1,})_([a-f0-9]{6,8})\}$`)

// legacyPlaceholderRegex 匹配旧格式 ${CRED_N}（向后兼容）
var legacyPlaceholderRegex = regexp.MustCompile(`^\$\{CRED_(\d+)\}$`)

// typeNameMap 标准化类型名映射（OneAIFW 可能返回不同大小写/格式）
var typeNameMap = map[string]string{
	"CRED":        "CRED",
	"PHONE":       "PHONE",
	"ID_CARD":     "ID_CARD",
	"EMAIL":       "EMAIL",
	"BANK_CARD":   "BANK_CARD",
	"IP_ADDRESS":  "IP_ADDRESS",
	"IP":          "IP_ADDRESS",
	"NAME":        "NAME",
	"ADDRESS":     "ADDRESS",
	"ORG":         "ORG",
	"LOCATION":    "LOCATION",
	"PERSON":      "PERSON",
	"COMPANY":     "COMPANY",
	"CREDENTIAL":  "CRED",
	"API_KEY":     "CRED",
	"JWT_TOKEN":   "CRED",
	"BEARER_TOKEN": "CRED",
}

// defaultGenerator 全局默认生成器（由 InitPlaceholderGenerator 初始化）
var defaultGenerator *PlaceholderGenerator

// NewPlaceholderGenerator 创建占位符生成器
func NewPlaceholderGenerator(hashLength int, hmacKey string) *PlaceholderGenerator {
	if hashLength < 6 {
		hashLength = 6
	}
	if hashLength > 8 {
		hashLength = 8
	}
	g := &PlaceholderGenerator{
		hashLength: hashLength,
	}
	if hmacKey != "" {
		g.hmacKey = []byte(hmacKey)
	}
	return g
}

// Generate 生成确定性占位符
// piType: PII 类型名（如 PHONE, CRED, EMAIL）
// value: 原始 PII 值
// 返回: ${TYPE_hash} 格式的占位符
func (g *PlaceholderGenerator) Generate(piType, value string) string {
	// 标准化类型名
	normalizedType := normalizeTypeName(piType)

	// 计算哈希
	var hashBytes []byte
	if len(g.hmacKey) > 0 {
		mac := hmac.New(sha256.New, g.hmacKey)
		mac.Write([]byte(value))
		hashBytes = mac.Sum(nil)
	} else {
		h := sha256.Sum256([]byte(value))
		hashBytes = h[:]
	}

	// 截取前 hashLength 位 hex
	hashStr := hex.EncodeToString(hashBytes)[:g.hashLength]

	return fmt.Sprintf("${%s_%s}", normalizedType, hashStr)
}

// Parse 解析占位符
// token: 占位符字符串（如 ${PHONE_a1b2c3}）
// 返回: (类型名, 哈希, 是否有效)
func (g *PlaceholderGenerator) Parse(token string) (piType, hash string, ok bool) {
	matches := placeholderRegex.FindStringSubmatch(token)
	if matches == nil {
		return "", "", false
	}
	return matches[1], matches[2], true
}

// ParseLegacy 解析旧格式占位符 ${CRED_N}
func (g *PlaceholderGenerator) ParseLegacy(token string) (piType string, index int, ok bool) {
	matches := legacyPlaceholderRegex.FindStringSubmatch(token)
	if matches == nil {
		return "", -1, false
	}
	var idx int
	for i, c := range matches[1] {
		idx = idx*10 + int(c-'0')
		_ = i
	}
	return "CRED", idx, true
}

// IsPlaceholderToken 判断字符串是否是有效占位符格式
func (g *PlaceholderGenerator) IsPlaceholderToken(s string) bool {
	if placeholderRegex.MatchString(s) {
		return true
	}
	// 向后兼容旧格式
	return legacyPlaceholderRegex.MatchString(s)
}

// normalizeTypeName 标准化类型名
func normalizeTypeName(typeName string) string {
	upper := strings.ToUpper(typeName)
	if mapped, ok := typeNameMap[upper]; ok {
		return mapped
	}
	// 未知类型：保持大写，替换非法字符为下划线
	result := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, upper)
	// 确保不以数字开头
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "T_" + result
	}
	return result
}

// ============ 全局便捷函数 ============

// InitPlaceholderGenerator 初始化全局占位符生成器
func InitPlaceholderGenerator(hashLength int, hmacKey string) {
	defaultGenerator = NewPlaceholderGenerator(hashLength, hmacKey)
}

// GeneratePlaceholder 生成确定性占位符（便捷函数，使用全局生成器）
func GeneratePlaceholder(piType, value string) string {
	if defaultGenerator == nil {
		defaultGenerator = NewPlaceholderGenerator(6, "")
	}
	return defaultGenerator.Generate(piType, value)
}

// ParsePlaceholder 解析占位符（便捷函数，使用全局生成器）
func ParsePlaceholder(token string) (piType, hash string, ok bool) {
	if defaultGenerator == nil {
		defaultGenerator = NewPlaceholderGenerator(6, "")
	}
	return defaultGenerator.Parse(token)
}

// IsPlaceholderToken 判断是否是有效占位符格式（便捷函数）
func IsPlaceholderToken(s string) bool {
	if defaultGenerator == nil {
		defaultGenerator = NewPlaceholderGenerator(6, "")
	}
	return defaultGenerator.IsPlaceholderToken(s)
}

// ContainsPlaceholder 检查文本中是否包含占位符（快速预检查）
func ContainsPlaceholder(text string) bool {
	return strings.Contains(text, "${")
}
