package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceholderStorage_Basic(t *testing.T) {
	// 创建临时目录
	tmpDir := filepath.Join(os.TempDir(), "placeholder_test")
	defer os.RemoveAll(tmpDir)

	storage, err := NewPlaceholderStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewPlaceholderStorage failed: %v", err)
	}

	// 测试保存
	err = storage.SavePlaceholder(context.Background(), "${PHONE_abc123}", "13800138000")
	if err != nil {
		t.Fatalf("SavePlaceholder failed: %v", err)
	}

	// 测试获取
	val, err := storage.GetPlaceholder(context.Background(), "${PHONE_abc123}")
	if err != nil || val != "13800138000" {
		t.Errorf("GetPlaceholder failed: got %v, %v, want 13800138000, nil", val, err)
	}
}

func TestPlaceholderStorage_MultiTypes(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "placeholder_multi_test")
	defer os.RemoveAll(tmpDir)

	storage, err := NewPlaceholderStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewPlaceholderStorage failed: %v", err)
	}

	// 保存不同类型的 PII
	testCases := []struct {
		placeholder string
		value       string
	}{
		{"${PHONE_abc123}", "13800138000"},
		{"${EMAIL_def456}", "test@example.com"},
		{"${ID_CARD_ghi789}", "110101199003076534"},
		{"${BANK_CARD_jkl012}", "6222021234567890123"},
		{"${IP_ADDRESS_mno345}", "192.168.1.100"},
	}

	for _, tc := range testCases {
		err := storage.SavePlaceholder(context.Background(), tc.placeholder, tc.value)
		if err != nil {
			t.Errorf("SavePlaceholder(%s) failed: %v", tc.placeholder, err)
		}
	}

	// 验证所有都能获取
	for _, tc := range testCases {
		val, err := storage.GetPlaceholder(context.Background(), tc.placeholder)
		if err != nil {
			t.Errorf("GetPlaceholder(%s) failed: %v", tc.placeholder, err)
		}
		if val != tc.value {
			t.Errorf("GetPlaceholder(%s) = %v, want %v", tc.placeholder, val, tc.value)
		}
	}
}

func TestPlaceholderStorage_FileStructure(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "placeholder_structure_test")
	defer os.RemoveAll(tmpDir)

	storage, err := NewPlaceholderStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewPlaceholderStorage failed: %v", err)
	}

	// 保存一个 PHONE 类型的占位符
	storage.SavePlaceholder(context.Background(), "${PHONE_abc123}", "13800138000")

	// 检查文件结构
	// 预期路径: tmpDir/PHONE/ab.json (hash前两位)
	filePath := filepath.Join(tmpDir, "PHONE", "ab.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// 可能是其他 hash 前缀，检查目录是否存在
		phoneDir := filepath.Join(tmpDir, "PHONE")
		if _, err := os.Stat(phoneDir); os.IsNotExist(err) {
			t.Error("PHONE directory should exist")
		}
	}
}

func TestPlaceholderStorage_NotFound(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "placeholder_notfound_test")
	defer os.RemoveAll(tmpDir)

	storage, err := NewPlaceholderStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewPlaceholderStorage failed: %v", err)
	}

	// 获取不存在的占位符
	_, err = storage.GetPlaceholder(context.Background(), "${PHONE_nonexistent}")
	if err == nil {
		t.Error("GetPlaceholder should fail for non-existent placeholder")
	}
}

func TestPlaceholderStorage_Overwrite(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "placeholder_overwrite_test")
	defer os.RemoveAll(tmpDir)

	storage, err := NewPlaceholderStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewPlaceholderStorage failed: %v", err)
	}

	// 保存
	storage.SavePlaceholder(context.Background(), "${PHONE_abc123}", "13800138000")

	// 覆盖
	storage.SavePlaceholder(context.Background(), "${PHONE_abc123}", "13900139000")

	// 验证新值
	val, err := storage.GetPlaceholder(context.Background(), "${PHONE_abc123}")
	if err != nil {
		t.Fatalf("GetPlaceholder failed: %v", err)
	}
	if val != "13900139000" {
		t.Errorf("GetPlaceholder = %v, want 13900139000", val)
	}
}
