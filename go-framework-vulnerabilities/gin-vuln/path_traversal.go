package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// 漏洞案例1: Gin框架路径遍历漏洞
// 这个漏洞展示了不当的文件路径处理可能导致的安全问题

// VulnerableFileHandler 存在路径遍历漏洞的文件处理器
func VulnerableFileHandler(c *gin.Context) {
	// 从URL参数获取文件名
	filename := c.Query("file")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file parameter is required"})
		return
	}

	// 漏洞点: 直接使用用户输入的文件名，没有进行安全检查
	filePath := fmt.Sprintf("./uploads/%s", filename)
	
	// 尝试打开文件
	file, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	defer file.Close()

	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	c.String(http.StatusOK, string(content))
}

// SafeFileHandler 修复后的安全文件处理器
func SafeFileHandler(c *gin.Context) {
	// 从URL参数获取文件名
	filename := c.Query("file")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file parameter is required"})
		return
	}

	// 安全检查1: 移除路径遍历字符
	filename = filepath.Base(filename)
	
	// 安全检查2: 验证文件名不包含危险字符
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	// 安全检查3: 使用白名单验证文件扩展名
	allowedExtensions := []string{".txt", ".pdf", ".jpg", ".png"}
	hasValidExtension := false
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(strings.ToLower(filename), ext) {
			hasValidExtension = true
			break
		}
	}
	if !hasValidExtension {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file type not allowed"})
		return
	}

	// 构建安全的文件路径
	uploadsDir, _ := filepath.Abs("./uploads")
	filePath := filepath.Join(uploadsDir, filename)
	
	// 安全检查4: 确保最终路径在uploads目录内
	if !strings.HasPrefix(filePath, uploadsDir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
		return
	}

	// 尝试打开文件
	file, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	defer file.Close()

	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	c.String(http.StatusOK, string(content))
}

// 漏洞案例2: 权限绕过漏洞
// 展示了不当的路由配置可能导致的权限绕过

// AuthMiddleware 简单的认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		// 简化的token验证
		if token != "Bearer valid-token" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminOnlyHandler 只有管理员才能访问的处理器
func AdminOnlyHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome admin!",
		"data":    "This is sensitive admin data",
	})
}

// SetupVulnerableRoutes 设置存在漏洞的路由
func SetupVulnerableRoutes(r *gin.Engine) {
	// 公开路由
	r.GET("/download", VulnerableFileHandler)
	
	// 漏洞点: 路由配置不当，可能被绕过
	adminGroup := r.Group("/admin")
	adminGroup.Use(AuthMiddleware())
	{
		adminGroup.GET("/panel", AdminOnlyHandler)
	}
	
	// 漏洞: 这个路由没有被保护，但处理相同的逻辑
	r.GET("/admin-panel", AdminOnlyHandler) // 忘记添加认证中间件！
}

// SetupSafeRoutes 设置安全的路由
func SetupSafeRoutes(r *gin.Engine) {
	// 公开路由 - 使用安全的文件处理器
	r.GET("/download", SafeFileHandler)
	
	// 管理员路由组 - 所有管理员路由都在这个组内
	adminGroup := r.Group("/admin")
	adminGroup.Use(AuthMiddleware())
	{
		adminGroup.GET("/panel", AdminOnlyHandler)
		// 所有管理员相关的路由都应该在这个组内
	}
	
	// 设置404处理器，防止路由探测
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
	})
}

func main() {
	// 创建uploads目录
	os.MkdirAll("./uploads", 0755)
	
	// 创建一些测试文件
	os.WriteFile("./uploads/test.txt", []byte("This is a test file"), 0644)
	os.WriteFile("./uploads/secret.txt", []byte("This is a secret file"), 0644)
	os.WriteFile("/etc/passwd_fake", []byte("root:x:0:0:root:/root:/bin/bash"), 0644)

	gin.SetMode(gin.ReleaseMode)
	
	fmt.Println("=== Gin框架路径遍历与权限绕过漏洞演示 ===")
	fmt.Println("\n1. 启动存在漏洞的服务器 (端口 8080)")
	fmt.Println("2. 启动安全的服务器 (端口 8081)")
	
	// 启动存在漏洞的服务器
	go func() {
		r := gin.New()
		SetupVulnerableRoutes(r)
		fmt.Println("\n[VULNERABLE] 服务器运行在 http://localhost:8080")
		r.Run(":8080")
	}()
	
	// 启动安全的服务器
	r := gin.New()
	SetupSafeRoutes(r)
	fmt.Println("[SAFE] 服务器运行在 http://localhost:8081")
	
	fmt.Println("\n漏洞测试方法:")
	fmt.Println("1. 路径遍历漏洞测试:")
	fmt.Println("   curl 'http://localhost:8080/download?file=../../../etc/passwd'")
	fmt.Println("   curl 'http://localhost:8080/download?file=test.txt'")
	fmt.Println("\n2. 权限绕过漏洞测试:")
	fmt.Println("   curl http://localhost:8080/admin-panel  # 无需认证即可访问!")
	fmt.Println("   curl http://localhost:8080/admin/panel  # 需要认证")
	fmt.Println("\n3. 安全版本测试:")
	fmt.Println("   curl 'http://localhost:8081/download?file=../../../etc/passwd'  # 被阻止")
	fmt.Println("   curl 'http://localhost:8081/download?file=test.txt'  # 正常访问")
	
	r.Run(":8081")
}