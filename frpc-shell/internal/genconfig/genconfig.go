// Package genconfig 生成并解析外壳托管的 frpc.toml 基线配置。
package genconfig

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// 基线模板（对应设计文档 5.2 节）。loginFailExit=false 与 store.path 是外壳可用的前提：
// 前者保证首次登录失败不退出进程，后者启用 /api/store/* 代理管理接口。
const tomlTemplate = `# frpc 配置文件（由 FrpcShell 生成）
# 只需填写 serverAddr 和 auth.token 两项，保存后从托盘菜单「启动服务」。

serverAddr = %q        # 必填：frps 服务器地址，例如 "frps.example.com"
serverPort = %d

# 服务器鉴权 token（与 frps 的 auth.token 保持一致）
auth.token = %q

# 断网或服务器未就绪时保持重试，外壳依赖此行为，请勿改为 true
loginFailExit = false

# 日志（「打开日志」菜单读取该文件）
log.to = "./logs/frpc.log"
log.level = "info"
log.maxDays = 7

# 管理接口：外壳与内置管理面板都依赖它，请勿删除
webServer.addr = "127.0.0.1"
webServer.port = %d
webServer.user = %q
webServer.password = %q

# 代理仓库：在管理面板里新增的代理会持久化到这个文件
store.path = "./store/frpc_store.json"
`

// TOML 返回基线配置内容。
func TOML(serverAddr string, serverPort int, token, adminUser, adminPassword string, adminPort int) string {
	return fmt.Sprintf(tomlTemplate,
		serverAddr, serverPort, token, adminPort, adminUser, adminPassword)
}

// RandomHex 生成 n 字节的随机十六进制串。
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败属于系统级异常，直接 panic 可接受（仅首次生成时调用）
		panic(fmt.Sprintf("read crypto/rand: %v", err))
	}
	return hex.EncodeToString(b)
}

var serverAddrRe = regexp.MustCompile(`(?m)^\s*serverAddr\s*=\s*"([^"]*)"`)

// ServerAddr 从配置内容中提取 serverAddr（仅匹配本外壳生成的简单行格式）。
func ServerAddr(content string) string {
	m := serverAddrRe.FindStringSubmatch(content)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
