// Package store 提供 JSON 文件的原子读写。
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Read 读取 JSON 文件到 v；文件不存在或损坏时保持 v 不变并返回 false。
func Read(path string, v any) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// Write 原子写入 JSON 文件（先写临时文件再改名）。
func Write(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
