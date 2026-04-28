//go:build cgo

package pii

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

// NERDetector NER 检测器（可选功能）
type NERDetector struct {
	enabled    bool
	modelDir   string
	models     map[string]string // language -> model name
	timeout    time.Duration
	mu         sync.RWMutex
	loaded     bool
	sessions   map[string]*ort.DynamicAdvancedSession // language -> session
	tokenizers map[string]*Tokenizer                  // language -> tokenizer
	labels     map[string][]string                    // language -> label list
}

// NERConfig NER 配置
type NERConfig struct {
	Enabled  bool              `json:"enabled"`
	ModelDir string            `json:"model_dir"`
	Models   map[string]string `json:"models"`  // language -> model name
	Timeout  time.Duration     `json:"timeout"` // 推理超时
}

// NEREntity NER 实体
type NEREntity struct {
	Type  string  // PER, ORG, LOC
	Value string  // 实体值
	Start int     // 起始位置
	End   int     // 结束位置
	Score float64 // 置信度
}

// Tokenizer 简单的分词器接口
type Tokenizer struct {
	vocab  map[string]int
	invVoc map[int]string
}

// NewTokenizer 创建分词器
func NewTokenizer(vocabPath string) (*Tokenizer, error) {
	data, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("读取词表失败: %w", err)
	}

	var vocab map[string]int
	if err := json.Unmarshal(data, &vocab); err != nil {
		return nil, fmt.Errorf("解析词表失败: %w", err)
	}

	invVoc := make(map[int]string)
	for k, v := range vocab {
		invVoc[v] = k
	}

	return &Tokenizer{vocab: vocab, invVoc: invVoc}, nil
}

// Tokenize 分词
func (t *Tokenizer) Tokenize(text string) []string {
	// 简单的字符级分词（适用于中文）
	// 实际应用中应使用与模型匹配的分词器
	tokens := []string{"[CLS]"}
	for _, r := range text {
		tokens = append(tokens, string(r))
	}
	tokens = append(tokens, "[SEP]")
	return tokens
}

// ConvertTokensToIDs 将 token 转换为 ID
func (t *Tokenizer) ConvertTokensToIDs(tokens []string) []int64 {
	ids := make([]int64, len(tokens))
	for i, token := range tokens {
		if id, ok := t.vocab[token]; ok {
			ids[i] = int64(id)
		} else {
			ids[i] = int64(t.vocab["[UNK]"]) // 未知词
		}
	}
	return ids
}

// NewNERDetector 创建 NER 检测器
func NewNERDetector(config NERConfig) *NERDetector {
	if config.Timeout <= 0 {
		config.Timeout = 100 * time.Millisecond
	}
	if config.ModelDir == "" {
		config.ModelDir = "./data/models"
	}
	if config.Models == nil {
		config.Models = map[string]string{
			"zh": "ckiplab/bert-tiny-chinese-ner",
			"en": "dslim/distilbert-NER",
		}
	}

	return &NERDetector{
		enabled:    config.Enabled,
		modelDir:   config.ModelDir,
		models:     config.Models,
		timeout:    config.Timeout,
		sessions:   make(map[string]*ort.DynamicAdvancedSession),
		tokenizers: make(map[string]*Tokenizer),
		labels:     make(map[string][]string),
	}
}

// Detect 检测文本中的 NER 实体
func (n *NERDetector) Detect(ctx context.Context, text string, language string) ([]NEREntity, error) {
	if !n.enabled {
		return nil, nil
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	// 检查模型是否已加载
	session, ok := n.sessions[language]
	if !ok {
		return nil, fmt.Errorf("模型未加载: %s", language)
	}

	tokenizer, ok := n.tokenizers[language]
	if !ok {
		return nil, fmt.Errorf("分词器未加载: %s", language)
	}

	labels, ok := n.labels[language]
	if !ok {
		return nil, fmt.Errorf("标签列表未加载: %s", language)
	}

	// 分词
	tokens := tokenizer.Tokenize(text)
	inputIDs := tokenizer.ConvertTokensToIDs(tokens)

	// 创建注意力掩码（全1）
	attentionMask := make([]int64, len(inputIDs))
	for i := range attentionMask {
		attentionMask[i] = 1
	}

	// 创建输入张量
	seqLen := int64(len(inputIDs))
	inputShape := ort.NewShape(1, seqLen)

	inputTensor, err := ort.NewTensor(inputShape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("创建输入张量失败: %w", err)
	}
	defer inputTensor.Destroy()

	attentionTensor, err := ort.NewTensor(inputShape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("创建注意力张量失败: %w", err)
	}
	defer attentionTensor.Destroy()

	// 创建输出张量
	outputShape := ort.NewShape(1, seqLen, int64(len(labels)))
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("创建输出张量失败: %w", err)
	}
	defer outputTensor.Destroy()

	// 运行推理
	err = session.Run(
		[]ort.Value{inputTensor, attentionTensor},
		[]ort.Value{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("推理失败: %w", err)
	}

	// 解析输出
	outputData := outputTensor.GetData()
	entities := n.parseOutput(outputData, tokens, labels, text)

	return entities, nil
}

// parseOutput 解析模型输出
func (n *NERDetector) parseOutput(output []float32, tokens []string, labels []string, originalText string) []NEREntity {
	var entities []NEREntity

	seqLen := len(tokens)
	numLabels := len(labels)

	// BIO 标签解析
	var currentEntity *NEREntity
	charOffset := 0

	for i := 1; i < seqLen-1; i++ { // 跳过 [CLS] 和 [SEP]
		// 找到最大概率的标签
		maxIdx := 0
		maxProb := float32(-1)
		for j := 0; j < numLabels; j++ {
			prob := output[i*numLabels+j]
			if prob > maxProb {
				maxProb = prob
				maxIdx = j
			}
		}

		label := labels[maxIdx]
		token := tokens[i]
		tokenLen := len(token)

		// 解析 BIO 标签
		switch {
		case label == "O":
			if currentEntity != nil {
				entities = append(entities, *currentEntity)
				currentEntity = nil
			}
		case len(label) > 2 && label[:2] == "B-":
			if currentEntity != nil {
				entities = append(entities, *currentEntity)
			}
			currentEntity = &NEREntity{
				Type:  label[2:],
				Value: token,
				Start: charOffset,
				End:   charOffset + tokenLen,
				Score: float64(maxProb),
			}
		case len(label) > 2 && label[:2] == "I-" && currentEntity != nil:
			currentEntity.Value += token
			currentEntity.End = charOffset + tokenLen
		}

		charOffset += tokenLen
	}

	if currentEntity != nil {
		entities = append(entities, *currentEntity)
	}

	return entities
}

// IsEnabled 检查是否启用
func (n *NERDetector) IsEnabled() bool {
	return n.enabled
}

// Enable 启用 NER
func (n *NERDetector) Enable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = true
}

// Disable 禁用 NER
func (n *NERDetector) Disable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = false
}

// LoadModel 加载模型
func (n *NERDetector) LoadModel(language string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	modelName, ok := n.models[language]
	if !ok {
		return fmt.Errorf("不支持的语言: %s", language)
	}

	modelPath := filepath.Join(n.modelDir, modelName, "onnx", "model.onnx")
	vocabPath := filepath.Join(n.modelDir, modelName, "vocab.json")
	labelsPath := filepath.Join(n.modelDir, modelName, "labels.json")

	// 检查模型文件是否存在
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("模型文件不存在: %s", modelPath)
	}

	// 加载分词器
	tokenizer, err := NewTokenizer(vocabPath)
	if err != nil {
		return fmt.Errorf("加载分词器失败: %w", err)
	}
	n.tokenizers[language] = tokenizer

	// 加载标签列表
	labelsData, err := os.ReadFile(labelsPath)
	if err != nil {
		return fmt.Errorf("读取标签文件失败: %w", err)
	}
	var labels []string
	if err := json.Unmarshal(labelsData, &labels); err != nil {
		return fmt.Errorf("解析标签文件失败: %w", err)
	}
	n.labels[language] = labels

	// 创建 ONNX 会话
	session, err := ort.NewDynamicAdvancedSession(modelPath, []string{"input_ids", "attention_mask"}, []string{"logits"}, nil)
	if err != nil {
		return fmt.Errorf("创建 ONNX 会话失败: %w", err)
	}
	n.sessions[language] = session

	n.loaded = true
	return nil
}

// UnloadModel 卸载模型
func (n *NERDetector) UnloadModel() {
	n.mu.Lock()
	defer n.mu.Unlock()

	for lang, session := range n.sessions {
		session.Destroy()
		delete(n.sessions, lang)
	}

	n.tokenizers = make(map[string]*Tokenizer)
	n.labels = make(map[string][]string)
	n.loaded = false
}

// Close 关闭检测器
func (n *NERDetector) Close() error {
	n.UnloadModel()
	return nil
}

// GetStats 获取统计信息
func (n *NERDetector) GetStats() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return map[string]interface{}{
		"enabled":  n.enabled,
		"loaded":   n.loaded,
		"modelDir": n.modelDir,
		"models":   n.models,
	}
}

// ============ 辅助函数 ============

// MapNERTypeToPIIType 映射 NER 类型到 PII 类型
func MapNERTypeToPIIType(nerType string) string {
	switch nerType {
	// 标准 NER 标签
	case "PER", "B-PER", "I-PER", "PERSON":
		return "USER_NAME"
	case "ORG", "B-ORG", "I-ORG", "ORGANIZATION":
		return "ORGANIZATION"
	case "LOC", "B-LOC", "I-LOC", "LOCATION", "GPE":
		return "PHYSICAL_ADDRESS"
	case "DATE", "TIME":
		return "DATE_TIME"

	// 中文模型常见标签 (gyr66/bert-base-chinese-finetuned-ner)
	case "name", "NAME":
		return "USER_NAME"
	case "address", "ADDRESS":
		return "PHYSICAL_ADDRESS"
	case "company", "COMPANY", "organization":
		return "ORGANIZATION"

	default:
		return ""
	}
}

// DetectAndMaskWithNER 使用 NER 检测并遮罩
func (n *NERDetector) DetectAndMaskWithNER(text string, language string, addPlaceholder func(placeholder, value string)) (string, error) {
	if !n.enabled {
		return text, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	entities, err := n.Detect(ctx, text, language)
	if err != nil {
		// NER 检测失败，返回原文
		return text, fmt.Errorf("NER 检测失败: %w", err)
	}

	if len(entities) == 0 {
		return text, nil
	}

	// 从后往前替换，避免索引偏移
	result := text
	for i := len(entities) - 1; i >= 0; i-- {
		entity := entities[i]
		piiType := MapNERTypeToPIIType(entity.Type)
		if piiType == "" {
			continue
		}

		placeholder := GeneratePlaceholder(piiType, entity.Value)

		// 通知调用方
		if addPlaceholder != nil {
			addPlaceholder(placeholder, entity.Value)
		}

		// 替换文本
		result = result[:entity.Start] + placeholder + result[entity.End:]
	}

	return result, nil
}

// InitializeONNX 初始化 ONNX Runtime 环境
func InitializeONNX(libPath string) error {
	ort.SetSharedLibraryPath(libPath)
	return ort.InitializeEnvironment()
}

// DestroyONNX 销毁 ONNX Runtime 环境
func DestroyONNX() {
	ort.DestroyEnvironment()
}
