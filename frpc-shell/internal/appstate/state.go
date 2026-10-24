// Package appstate 管理外壳自身状态文件 data/shell.json。
package appstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// State 是外壳的持久化状态（见设计文档 5.3 节）。
// 代理配置不在这里：配置事实源只有 frpc.toml 与 store 文件。
type State struct {
	FrpcVersion   string `json:"frpcVersion"`
	AdminPort     int    `json:"adminPort"`
	AdminUser     string `json:"adminUser"`
	AdminPassword string `json:"adminPassword"`
	Autostart     bool   `json:"autostart"`
	Onboarded     bool   `json:"onboarded"`
}

// Default 返回首次运行的默认状态。password 由调用方生成。
func Default(password string) *State {
	return &State{
		AdminPort:     7400,
		AdminUser:     "admin",
		AdminPassword: password,
	}
}

func Load(path string) (*State, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &s, nil
}

func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
