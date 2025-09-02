# Go框架罕见漏洞研究项目

本项目深入研究了Go语言流行框架（Gin和XORM）中的罕见安全漏洞，通过代码复现和详细分析，帮助开发者理解和防范这些安全威胁。

## 项目结构

```
go-framework-vulnerabilities/
├── gin-vuln/                      # Gin框架漏洞示例
│   ├── path_traversal.go         # 路径遍历和权限绕过漏洞
│   └── race_condition.go         # 竞态条件漏洞
├── xorm-vuln/                     # XORM框架漏洞示例
│   └── second_order_injection.go # 二阶SQL注入漏洞
├── docs/                          # 文档
│   └── vulnerability_analysis.md  # 详细的漏洞分析报告
├── test_vulnerabilities.sh        # 漏洞测试脚本
├── go.mod                         # Go模块文件
└── README.md                      # 本文件
```

## 研究的漏洞

### 1. Gin框架路径遍历漏洞
- **描述**：不当的文件路径处理导致攻击者可以访问应用目录外的文件
- **影响**：可能泄露系统敏感文件（如/etc/passwd）
- **文件**：`gin-vuln/path_traversal.go`

### 2. Gin框架权限绕过漏洞
- **描述**：路由配置不当导致认证中间件被绕过
- **影响**：未授权用户可以访问管理功能
- **文件**：`gin-vuln/path_traversal.go`

### 3. Gin框架竞态条件漏洞
- **描述**：并发访问共享资源时缺少适当的同步机制
- **影响**：可能导致数据不一致，如余额异常
- **文件**：`gin-vuln/race_condition.go`

### 4. XORM框架二阶SQL注入漏洞
- **描述**：恶意数据先被存储，后在其他操作中触发SQL注入
- **影响**：可能导致数据泄露或数据库被破坏
- **文件**：`xorm-vuln/second_order_injection.go`

## 快速开始

### 环境要求
- Go 1.21 或更高版本
- SQLite3（用于XORM示例）

### 安装依赖
```bash
cd go-framework-vulnerabilities
go mod download
```

### 运行示例

1. **路径遍历和权限绕过漏洞演示**：
```bash
go run gin-vuln/path_traversal.go
```

2. **竞态条件漏洞演示**：
```bash
go run gin-vuln/race_condition.go
```

3. **二阶SQL注入漏洞演示**：
```bash
go run xorm-vuln/second_order_injection.go
```

### 测试漏洞

运行测试脚本：
```bash
./test_vulnerabilities.sh
```

## 主要发现

1. **框架使用不当比框架本身的漏洞更常见**：大多数漏洞源于开发者对框架的错误使用，而非框架本身的缺陷。

2. **输入验证至关重要**：几乎所有漏洞都与未充分验证用户输入有关。

3. **并发安全需要特别注意**：Go的并发特性虽然强大，但也带来了额外的安全挑战。

4. **二阶攻击更难防范**：攻击payload可能通过看似安全的操作进入系统，在后续操作中被触发。

## 修复建议

1. **路径遍历防护**：
   - 使用`filepath.Base()`提取文件名
   - 验证文件扩展名白名单
   - 确保最终路径在预期目录内

2. **权限控制**：
   - 统一管理受保护的路由
   - 使用中间件进行认证和授权
   - 遵循最小权限原则

3. **并发安全**：
   - 使用互斥锁保护共享资源
   - 考虑使用通道（channel）进行通信
   - 实施适当的请求限流

4. **SQL注入防护**：
   - 始终使用参数化查询
   - 即使数据来自数据库也要进行验证
   - 限制数据库用户权限

## 深入学习

详细的漏洞分析和心得体会请参阅：[vulnerability_analysis.md](docs/vulnerability_analysis.md)

## 安全建议

1. **定期更新依赖**：保持框架和依赖库的版本更新
2. **代码审计**：定期进行安全代码审计
3. **安全测试**：在CI/CD流程中加入安全测试
4. **培训团队**：确保团队成员了解常见的安全威胁

## 免责声明

本项目仅用于教育和研究目的。请勿将这些漏洞利用技术用于非法用途。使用本项目代码进行的任何行为，使用者需自行承担责任。

## 贡献

欢迎提交Issue和Pull Request，分享更多的安全漏洞案例和防护方法。

## 许可证

MIT License