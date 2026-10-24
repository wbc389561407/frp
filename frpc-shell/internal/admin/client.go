// Package admin 封装 frpc Admin API（设计文档 4.3 节）。
// 错误响应体为 {"Code": <int>, "Msg": "<string>"}，注意字段名首字母大写。
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base   string
	user   string
	pwd    string
	client *http.Client
}

func New(port int, user, pwd string) *Client {
	return &Client{
		base:   fmt.Sprintf("http://127.0.0.1:%d", port),
		user:   user,
		pwd:    pwd,
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

// ProxyStatus 对应 GET /api/status 中单个代理条目。
type ProxyStatus struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Err        string `json:"err"`
	LocalAddr  string `json:"local_addr"`
	Plugin     string `json:"plugin"`
	RemoteAddr string `json:"remote_addr"`
	Source     string `json:"source"`
}

// Healthz 探活端点，无认证。用于受管启动后的就绪判定。
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: status %d", resp.StatusCode)
	}
	return nil
}

// Status 返回按类型分组的全量代理状态。
func (c *Client) Status(ctx context.Context) (map[string][]ProxyStatus, error) {
	body, err := c.do(ctx, http.MethodGet, "/api/status", "")
	if err != nil {
		return nil, err
	}
	var res map[string][]ProxyStatus
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return nil, fmt.Errorf("parse /api/status: %w", err)
	}
	return res, nil
}

// Stop 触发优雅停止（frpc 侧 100ms 宽限后自行退出）。
func (c *Client) Stop(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodPost, "/api/stop", "")
	return err
}

func (c *Client) do(ctx context.Context, method, path, body string) (string, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return "", err
	}
	if c.user != "" || c.pwd != "" {
		req.SetBasicAuth(c.user, c.pwd)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		// frpc 错误体格式 {"Code":..,"Msg":..}，只取 Msg 展示
		var e struct {
			Code int    `json:"Code"`
			Msg  string `json:"Msg"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Msg != "" {
			return "", fmt.Errorf("api %s: %d %s", path, resp.StatusCode, e.Msg)
		}
		return "", fmt.Errorf("api %s: status %d", path, resp.StatusCode)
	}
	return string(raw), nil
}
