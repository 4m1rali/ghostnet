# 为 GhostNet 做贡献

## 开始

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build ./...
```

---

## 为所有平台构建

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-amd64 ./cmd/ghostnet/

# Linux arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-arm64 ./cmd/ghostnet/
```

---

## 添加绕过策略

1. 创建 `internal/bypass/mymethod.go`
2. 实现注入函数
3. 在 `adaptive.go` 中添加新的 `Strategy` 常量
4. 将 case 添加到 `AdaptiveBypass.run()`
5. 将其添加到 `AdaptiveBypass.next()` 中的回退链

---

## 将 SNI 域名添加到内置列表

编辑 `internal/preflight/checker.go`：

```go
var KnownSNIDomains = []string{
    "hcaptcha.com",
    // 在此处添加您的域名
}
```

域名应该：
- 在大多数网络上可通过端口 443 访问
- 在常见的 DPI 白名单中（CDN 域名、主要服务）
- 在目标地区未被封锁

---

## Pull Request 检查清单

- [ ] `go build ./...` 通过
- [ ] 没有新的外部依赖
- [ ] 平台特定代码有构建标签
- [ ] 新的绕过策略已添加到 adaptive 回退链
- [ ] 新的 config 字段在 `config.Default()` 中有默认值

---

## 报告问题

请附上：
- 操作系统和内核版本（`uname -a`）
- GhostNet 版本（`./ghostnet version`）
- 完整错误输出
- config 文件（删除敏感 IP）
- 网络环境（ISP、国家、VPS 提供商）
