package pii

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewModelManager(t *testing.T) {
	config := ModelConfig{
		ModelDir: "./test_models",
		Proxy:    "",
	}

	mgr := NewModelManager(config)

	if mgr.modelDir != "./test_models" {
		t.Errorf("Expected modelDir to be './test_models', got %s", mgr.modelDir)
	}
}

func TestModelManagerGetModelPath(t *testing.T) {
	mgr := NewModelManager(ModelConfig{ModelDir: "./test_models"})

	path := mgr.GetModelPath("ckiplab/bert-tiny-chinese-ner")
	expected := filepath.Join("./test_models", "ckiplab_bert-tiny-chinese-ner")

	if path != expected {
		t.Errorf("GetModelPath() = %s, want %s", path, expected)
	}
}

func TestModelManagerIsModelDownloaded(t *testing.T) {
	mgr := NewModelManager(ModelConfig{ModelDir: "./test_models"})

	// 不存在的模型
	if mgr.IsModelDownloaded("nonexistent/model") {
		t.Error("Expected IsModelDownloaded to return false for nonexistent model")
	}
}

func TestModelManagerListModels(t *testing.T) {
	mgr := NewModelManager(ModelConfig{ModelDir: "./test_models"})

	models, err := mgr.ListModels()
	if err != nil {
		t.Errorf("ListModels() error = %v", err)
	}

	// 空目录应该返回空列表
	if len(models) != 0 {
		t.Errorf("Expected empty list, got %d models", len(models))
	}
}

func TestModelManagerDeleteModel(t *testing.T) {
	mgr := NewModelManager(ModelConfig{ModelDir: "./test_models_delete"})

	// 创建一个临时模型目录
	modelDir := filepath.Join("./test_models_delete", "test_model")
	onnxDir := filepath.Join(modelDir, "onnx")
	os.MkdirAll(onnxDir, 0755)

	// 创建一个假的 onnx 文件
	onnxPath := filepath.Join(onnxDir, "model_quantized.onnx")
	os.WriteFile(onnxPath, []byte("fake model"), 0644)

	// 验证模型存在
	if !mgr.IsModelDownloaded("test_model") {
		t.Error("Expected model to exist")
	}

	// 删除模型
	err := mgr.DeleteModel("test_model")
	if err != nil {
		t.Errorf("DeleteModel() error = %v", err)
	}

	// 验证模型已删除
	if mgr.IsModelDownloaded("test_model") {
		t.Error("Expected model to be deleted")
	}

	// 清理
	os.RemoveAll("./test_models_delete")
}

func TestModelManagerDownloadModel(t *testing.T) {
	// 这个测试需要网络连接，跳过
	t.Skip("需要网络连接，跳过实际下载测试")

	// 如果需要测试，可以创建一个 mock HTTP server
	// 或者使用测试 fixture
}

func TestGenerateLabelsFromConfig(t *testing.T) {
	mgr := NewModelManager(ModelConfig{ModelDir: "./test_models_labels"})

	// 创建测试目录
	os.MkdirAll("./test_models_labels", 0755)

	// 创建 config.json
	configPath := filepath.Join("./test_models_labels", "config.json")
	configContent := `{
		"id2label": {
			"0": "O",
			"1": "B-PER",
			"2": "I-PER",
			"3": "B-ORG",
			"4": "I-ORG",
			"5": "B-LOC",
			"6": "I-LOC"
		}
	}`
	os.WriteFile(configPath, []byte(configContent), 0644)

	// 生成 labels.json
	labelsPath := filepath.Join("./test_models_labels", "labels.json")
	err := mgr.generateLabelsFromConfig(configPath, labelsPath)
	if err != nil {
		t.Errorf("generateLabelsFromConfig() error = %v", err)
	}

	// 验证 labels.json
	data, err := os.ReadFile(labelsPath)
	if err != nil {
		t.Errorf("读取 labels.json 失败: %v", err)
	}

	expected := `["O","B-PER","I-PER","B-ORG","I-ORG","B-LOC","I-LOC"]`
	if string(data) != expected {
		t.Errorf("labels.json = %s, want %s", string(data), expected)
	}

	// 清理
	os.RemoveAll("./test_models_labels")
}
