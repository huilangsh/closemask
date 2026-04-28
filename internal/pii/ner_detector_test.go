package pii

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewNERDetector(t *testing.T) {
	config := NERConfig{
		Enabled:  true,
		ModelDir: "./test_models",
		Timeout:  50 * time.Millisecond,
	}

	detector := NewNERDetector(config)

	if !detector.IsEnabled() {
		t.Error("Expected detector to be enabled")
	}

	stats := detector.GetStats()
	if stats["enabled"] != true {
		t.Error("Expected enabled to be true in stats")
	}
}

func TestNERDetectorDisable(t *testing.T) {
	detector := NewNERDetector(NERConfig{Enabled: true})

	detector.Disable()
	if detector.IsEnabled() {
		t.Error("Expected detector to be disabled")
	}

	detector.Enable()
	if !detector.IsEnabled() {
		t.Error("Expected detector to be enabled")
	}
}

func TestNERDetectorDetectDisabled(t *testing.T) {
	detector := NewNERDetector(NERConfig{Enabled: false})

	ctx := context.Background()
	entities, err := detector.Detect(ctx, "测试文本", "zh")

	if err != nil {
		t.Errorf("Expected no error when disabled, got: %v", err)
	}
	if entities != nil {
		t.Error("Expected nil entities when disabled")
	}
}

func TestMapNERTypeToPIIType(t *testing.T) {
	tests := []struct {
		nerType  string
		expected string
	}{
		{"PER", "USER_NAME"},
		{"B-PER", "USER_NAME"},
		{"I-PER", "USER_NAME"},
		{"ORG", "ORGANIZATION"},
		{"B-ORG", "ORGANIZATION"},
		{"LOC", "PHYSICAL_ADDRESS"},
		{"B-LOC", "PHYSICAL_ADDRESS"},
		{"O", ""},
		{"MISC", ""},
	}

	for _, tt := range tests {
		result := MapNERTypeToPIIType(tt.nerType)
		if result != tt.expected {
			t.Errorf("MapNERTypeToPIIType(%s) = %s, want %s", tt.nerType, result, tt.expected)
		}
	}
}

func TestDetectAndMaskWithNERDisabled(t *testing.T) {
	detector := NewNERDetector(NERConfig{Enabled: false})

	text := "测试文本"
	result, err := detector.DetectAndMaskWithNER(text, "zh", nil)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != text {
		t.Errorf("Expected original text when disabled")
	}
}

func TestNERDetectorClose(t *testing.T) {
	detector := NewNERDetector(NERConfig{Enabled: true})

	err := detector.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestNERDetectorLoadModelNonExistent(t *testing.T) {
	detector := NewNERDetector(NERConfig{
		Enabled:  true,
		ModelDir: "./nonexistent_models",
	})

	err := detector.LoadModel("zh")
	if err == nil {
		t.Error("Expected error when loading non-existent model")
	}
}

// TestNERDetectorWithCGO 测试 CGO 环境下的 NER 推理
// 需要安装 GCC 和 ONNX Runtime 共享库
func TestNERDetectorWithCGO(t *testing.T) {
	// 检查是否有 CGO 支持
	stats := NewNERDetector(NERConfig{Enabled: true}).GetStats()
	if cgo, ok := stats["cgo"]; ok && cgo == false {
		t.Skip("CGO 不可用，跳过 ONNX 推理测试")
	}

	// 检查模型是否存在
	modelDir := "./data/models"
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		t.Skip("模型目录不存在，跳过 ONNX 推理测试")
	}

	// 检查 ONNX Runtime 共享库
	// Windows: onnxruntime.dll
	// Linux: libonnxruntime.so
	// macOS: libonnxruntime.dylib
	dllPaths := []string{
		"onnxruntime.dll",
		"libonnxruntime.so",
		"libonnxruntime.dylib",
	}
	found := false
	for _, path := range dllPaths {
		if _, err := os.Stat(path); err == nil {
			found = true
			break
		}
	}
	if !found {
		t.Skip("ONNX Runtime 共享库不存在，跳过 ONNX 推理测试")
	}

	// 初始化 ONNX Runtime
	err := InitializeONNX("onnxruntime.dll")
	if err != nil {
		t.Fatalf("InitializeONNX() error = %v", err)
	}
	defer DestroyONNX()

	// 创建检测器
	detector := NewNERDetector(NERConfig{
		Enabled:  true,
		ModelDir: modelDir,
		Timeout:  100 * time.Millisecond,
	})

	// 加载模型
	err = detector.LoadModel("zh")
	if err != nil {
		t.Fatalf("LoadModel() error = %v", err)
	}
	defer detector.Close()

	// 测试推理
	ctx := context.Background()
	text := "张三在北京工作，他的公司是腾讯。"
	entities, err := detector.Detect(ctx, text, "zh")
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	// 验证结果
	if len(entities) == 0 {
		t.Log("未检测到实体（可能模型未正确加载）")
	} else {
		t.Logf("检测到 %d 个实体:", len(entities))
		for _, e := range entities {
			t.Logf("  - %s: %s (%.2f)", e.Type, e.Value, e.Score)
		}
	}
}

// TestNERDetectorIntegration 集成测试：完整的 NER 遮罩流程
func TestNERDetectorIntegration(t *testing.T) {
	// 检查是否有 CGO 支持
	stats := NewNERDetector(NERConfig{Enabled: true}).GetStats()
	if cgo, ok := stats["cgo"]; ok && cgo == false {
		t.Skip("CGO 不可用，跳过集成测试")
	}

	// 初始化
	InitializeONNX("onnxruntime.dll")
	defer DestroyONNX()

	detector := NewNERDetector(NERConfig{
		Enabled:  true,
		ModelDir: "./data/models",
		Timeout:  100 * time.Millisecond,
	})

	err := detector.LoadModel("zh")
	if err != nil {
		t.Skipf("LoadModel() error = %v", err)
	}
	defer detector.Close()

	// 测试遮罩
	text := "李四的邮箱是 lisi@example.com，住址是上海市浦东新区。"
	placeholders := make(map[string]string)
	addPlaceholder := func(placeholder, value string) {
		placeholders[placeholder] = value
	}

	result, err := detector.DetectAndMaskWithNER(text, "zh", addPlaceholder)
	if err != nil {
		t.Fatalf("DetectAndMaskWithNER() error = %v", err)
	}

	t.Logf("原文: %s", text)
	t.Logf("遮罩后: %s", result)
	t.Logf("占位符: %v", placeholders)
}
