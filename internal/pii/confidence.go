package pii

// MaskLevel 置信度级别
type MaskLevel string

const (
	// MaskLevelStrict 严格模式：仅高置信度（≥0.8），几乎不误报
	MaskLevelStrict MaskLevel = "strict"
	// MaskLevelBalanced 平衡模式：中等置信度（≥0.6），推荐默认
	MaskLevelBalanced MaskLevel = "balanced"
	// MaskLevelAggressive 激进模式：低置信度（≥0.4），可能误报
	MaskLevelAggressive MaskLevel = "aggressive"
)

// 置信度阈值映射
var levelThresholds = map[MaskLevel]float64{
	MaskLevelStrict:     0.8,
	MaskLevelBalanced:   0.6,
	MaskLevelAggressive: 0.4,
}

// PII 类型的默认置信度
var typeConfidence = map[string]float64{
	// 原有类型
	"ID_CARD":   0.90,
	"BANK_CARD": 0.80,
	"PHONE":     0.90,
	"EMAIL":     0.90,
	"IP_ADDRESS": 0.80,

	// 新增类型
	"URL_ADDRESS":      0.80,
	"PRIVATE_KEY":      0.95,
	"VERIFICATION_CODE": 0.80,
	"PASSWORD":         0.60,
	"RANDOM_SEED":      0.70,

	// NER 检测类型（可选）
	"USER_NAME":        0.70,
	"ORGANIZATION":     0.70,
	"PHYSICAL_ADDRESS": 0.70,
}

// GetThreshold 获取置信度阈值
func GetThreshold(level MaskLevel) float64 {
	if threshold, ok := levelThresholds[level]; ok {
		return threshold
	}
	return 0.6 // 默认 balanced
}

// GetTypeConfidence 获取 PII 类型的置信度
func GetTypeConfidence(typeName string) float64 {
	if confidence, ok := typeConfidence[typeName]; ok {
		return confidence
	}
	return 0.5 // 默认中等置信度
}

// ShouldMask 判断是否应该遮罩
func ShouldMask(typeName string, level MaskLevel) bool {
	confidence := GetTypeConfidence(typeName)
	threshold := GetThreshold(level)
	return confidence >= threshold
}

// ParseMaskLevel 解析置信度级别
func ParseMaskLevel(s string) MaskLevel {
	switch s {
	case "strict":
		return MaskLevelStrict
	case "aggressive":
		return MaskLevelAggressive
	default:
		return MaskLevelBalanced
	}
}
