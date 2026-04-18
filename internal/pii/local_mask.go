package pii

import (
	"regexp"
	"strings"
)

// Pattern 通用匹配模式（合并 KeyNamePattern / ValueFormatPattern）
type Pattern struct {
	Regex      *regexp.Regexp
	ValueGroup int
}

// LocalMasker 凭据本地预扫描器
type LocalMasker struct {
	keyPatterns []Pattern
	valPatterns []Pattern
	level       string
}

// NewLocalMasker 创建本地预扫描器
func NewLocalMasker(level string) *LocalMasker {
	if level == "" {
		level = "strict"
	}
	lm := &LocalMasker{level: level}
	if level == "off" {
		return lm
	}
	lm.initKeyPatterns()
	if level == "aggressive" {
		lm.initAggressiveValPatterns()
	} else {
		lm.initStrictValPatterns()
	}
	return lm
}

func (lm *LocalMasker) initKeyPatterns() {
	keyNames := []string{
		// 精确模式（优先匹配，放在前面）
		`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DASHSCOPE_API_KEY`, `ZHIPU_API_KEY`,
		`DEEPSEEK_API_KEY`, `QIANFAN_ACCESS_KEY`, `QIANFAN_SECRET_KEY`,
		`SPARK_API_KEY`, `MOONSHOT_API_KEY`, `MINIMAX_API_KEY`,
		`HUNYUAN_API_KEY`, `BAICHUAN_API_KEY`, `YI_API_KEY`,
		`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`,
		`GITHUB_TOKEN`, `GITLAB_TOKEN`, `GH_TOKEN`,
		`DATABASE_URL`, `DB_PASSWORD`, `MYSQL_PASSWORD`, `REDIS_PASSWORD`,
		`MONGO_PASSWORD`, `POSTGRES_PASSWORD`,
		// 通用模式（放后面）
		`api[_\-]?key`, `apikey`, `secret[_\-]?key`, `secretkey`,
		`access[_\-]?key`, `access[_\-]?token`, `auth[_\-]?token`,
		`private[_\-]?key`, `password`, `passwd`, `pwd`,
		// HTTP Headers
		`Authorization`, `X[_\-]?API[_\-]?Key`, `Proxy[_\-]?Authorization`,
		// 通用 token 放最后（最宽泛）
		`token`, `credential`,
	}

	for _, kn := range keyNames {
		// 支持多种格式：
		//   JSON: "apiKey": "xxx", "apiKey":"xxx"
		//   裸键名: apiKey=xxx, apiKey: xxx, OPENAI_API_KEY=xxx
		//   带引号裸键名: apiKey = "xxx"
		// 键名可能有可选的前后引号，后面跟 : 或 =，再跟可选引号包围的值
		pattern := `(?i)(?:["']?` + kn + `["']?\s*[:=]\s*["']?)([^\s"']{8,})["']?`
		re := regexp.MustCompile(pattern)
		lm.keyPatterns = append(lm.keyPatterns, Pattern{
			Regex:      re,
			ValueGroup: 1,
		})
	}
}

func (lm *LocalMasker) initStrictValPatterns() {
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`Bearer\s+(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)`),
		ValueGroup: 1,
	})
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`\b(AKIA[A-Z0-9]{16})\b`),
		ValueGroup: 1,
	})
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`(?i)(?:postgres|mongodb|mysql|redis|amqp)://[^:]+:([^@]+)@`),
		ValueGroup: 1,
	})
}

func (lm *LocalMasker) initAggressiveValPatterns() {
	lm.initStrictValPatterns()
	// 常见 LLM/API Key 前缀（sk-, pk-, rk-, gk-, p- 等），允许值中包含连字符
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`\b([sprg]k-[A-Za-z0-9_-]{20,})\b`),
		ValueGroup: 1,
	})
	// UUID 格式（8-4-4-4-12），常见于 API Key 和凭据
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`\b([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})\b`),
		ValueGroup: 1,
	})
	// pk- 前缀 + UUID 格式（如 pk-1148a252-026e-4aa3-a69f-ab16eef56a3e）
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`\b(pk-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})\b`),
		ValueGroup: 1,
	})
	// 纯字母数字长字符串（40+）
	lm.valPatterns = append(lm.valPatterns, Pattern{
		Regex:      regexp.MustCompile(`\b([A-Za-z0-9]{40,})\b`),
		ValueGroup: 1,
	})
}

// Mask 对文本进行本地预扫描遮罩
func (lm *LocalMasker) Mask(text string, addPlaceholder func(placeholder, value string)) string {
	if lm.level == "off" || text == "" {
		return text
	}
	result := lm.maskByKeyNames(text, addPlaceholder)
	result = lm.maskByValueFormats(result, addPlaceholder)
	return result
}

// maskByKeyNames 键名匹配遮罩
func (lm *LocalMasker) maskByKeyNames(text string, addPlaceholder func(placeholder, value string)) string {
	result := text
	for _, kp := range lm.keyPatterns {
		matches := kp.Regex.FindAllStringSubmatchIndex(result, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			valueStart := m[kp.ValueGroup*2]
			valueEnd := m[kp.ValueGroup*2+1]
			if valueStart < 0 || valueEnd < 0 {
				continue
			}
			originalValue := result[valueStart:valueEnd]
			if IsPlaceholder(originalValue) {
				continue
			}
			placeholder := GeneratePlaceholder("CRED", originalValue)
			addPlaceholder(placeholder, originalValue)
			result = result[:valueStart] + placeholder + result[valueEnd:]
		}
	}
	return result
}

// maskByValueFormats 值格式匹配遮罩
func (lm *LocalMasker) maskByValueFormats(text string, addPlaceholder func(placeholder, value string)) string {
	result := text
	for _, vp := range lm.valPatterns {
		matches := vp.Regex.FindAllStringSubmatchIndex(result, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			valueStart := m[vp.ValueGroup*2]
			valueEnd := m[vp.ValueGroup*2+1]
			if valueStart < 0 || valueEnd < 0 {
				continue
			}
			originalValue := result[valueStart:valueEnd]
			if IsPlaceholder(originalValue) {
				continue
			}
			if isInsidePlaceholder(result, valueStart) {
				continue
			}
			placeholder := GeneratePlaceholder("CRED", originalValue)
			addPlaceholder(placeholder, originalValue)
			result = result[:valueStart] + placeholder + result[valueEnd:]
		}
	}
	return result
}

// isInsidePlaceholder 检查位置是否在占位符内部
func isInsidePlaceholder(text string, pos int) bool {
	if pos <= 2 {
		return false
	}
	// 找最近的 ${
	leftIdx := strings.LastIndex(text[:pos], "${")
	if leftIdx == -1 || leftIdx < pos-60 {
		return false
	}
	// 找最近的 }
	rightIdx := strings.Index(text[leftIdx:], "}")
	if rightIdx == -1 {
		return false
	}
	candidate := text[leftIdx : leftIdx+rightIdx+1]
	// 验证是否是有效占位符
	if IsPlaceholderToken(candidate) {
		return pos >= leftIdx && pos <= leftIdx+rightIdx+1
	}
	return false
}
