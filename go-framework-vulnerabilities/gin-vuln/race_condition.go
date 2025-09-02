package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 漏洞案例3: Gin框架中的竞态条件漏洞
// 展示了并发访问共享资源时可能出现的安全问题

// Account 账户结构
type Account struct {
	ID      string
	Balance float64
	mu      sync.Mutex // 用于安全版本
}

// VulnerableTransferService 存在竞态条件漏洞的转账服务
type VulnerableTransferService struct {
	accounts map[string]*Account
	// 注意：没有使用互斥锁保护
}

// SafeTransferService 安全的转账服务
type SafeTransferService struct {
	accounts map[string]*Account
	mu       sync.RWMutex // 保护accounts map
}

// VulnerableTransfer 存在竞态条件的转账方法
func (s *VulnerableTransferService) VulnerableTransfer(c *gin.Context) {
	var req struct {
		From   string  `json:"from" binding:"required"`
		To     string  `json:"to" binding:"required"`
		Amount float64 `json:"amount" binding:"required,gt=0"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// 漏洞：没有适当的同步机制
	fromAccount, exists := s.accounts[req.From]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "source account not found"})
		return
	}
	
	toAccount, exists := s.accounts[req.To]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "destination account not found"})
		return
	}
	
	// 竞态条件漏洞：检查余额和扣款之间存在时间窗口
	if fromAccount.Balance < req.Amount {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient balance"})
		return
	}
	
	// 模拟处理延迟，增加竞态条件发生的概率
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
	
	// 执行转账（没有原子性保证）
	fromAccount.Balance -= req.Amount
	toAccount.Balance += req.Amount
	
	c.JSON(http.StatusOK, gin.H{
		"message": "transfer successful",
		"from_balance": fromAccount.Balance,
		"to_balance": toAccount.Balance,
	})
}

// SafeTransfer 安全的转账方法
func (s *SafeTransferService) SafeTransfer(c *gin.Context) {
	var req struct {
		From   string  `json:"from" binding:"required"`
		To     string  `json:"to" binding:"required"`
		Amount float64 `json:"amount" binding:"required,gt=0"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// 使用读锁获取账户
	s.mu.RLock()
	fromAccount, fromExists := s.accounts[req.From]
	toAccount, toExists := s.accounts[req.To]
	s.mu.RUnlock()
	
	if !fromExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "source account not found"})
		return
	}
	
	if !toExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "destination account not found"})
		return
	}
	
	// 按账户ID顺序加锁，避免死锁
	if req.From < req.To {
		fromAccount.mu.Lock()
		defer fromAccount.mu.Unlock()
		toAccount.mu.Lock()
		defer toAccount.mu.Unlock()
	} else {
		toAccount.mu.Lock()
		defer toAccount.mu.Unlock()
		fromAccount.mu.Lock()
		defer fromAccount.mu.Unlock()
	}
	
	// 在锁保护下检查余额
	if fromAccount.Balance < req.Amount {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient balance"})
		return
	}
	
	// 原子性地执行转账
	fromAccount.Balance -= req.Amount
	toAccount.Balance += req.Amount
	
	c.JSON(http.StatusOK, gin.H{
		"message": "transfer successful",
		"from_balance": fromAccount.Balance,
		"to_balance": toAccount.Balance,
	})
}

// VulnerableRateLimiter 存在竞态条件的限流器
type VulnerableRateLimiter struct {
	requests map[string]int
	window   time.Duration
	limit    int
}

// VulnerableCheckLimit 检查是否超过限制（有竞态条件）
func (r *VulnerableRateLimiter) VulnerableCheckLimit(clientIP string) bool {
	// 漏洞：对map的并发访问没有保护
	count, exists := r.requests[clientIP]
	if !exists {
		r.requests[clientIP] = 1
		// 设置过期清理
		go func() {
			time.Sleep(r.window)
			// 竞态条件：可能同时删除
			delete(r.requests, clientIP)
		}()
		return true
	}
	
	if count >= r.limit {
		return false
	}
	
	// 竞态条件：读取和写入之间可能有其他goroutine修改
	r.requests[clientIP]++
	return true
}

// SafeRateLimiter 安全的限流器
type SafeRateLimiter struct {
	requests sync.Map
	window   time.Duration
	limit    int
}

// SafeCheckLimit 安全的限流检查
func (r *SafeRateLimiter) SafeCheckLimit(clientIP string) bool {
	now := time.Now()
	
	// 使用LoadOrStore原子操作
	val, _ := r.requests.LoadOrStore(clientIP, &struct {
		count      int
		resetTime  time.Time
		mu         sync.Mutex
	}{
		count:     0,
		resetTime: now.Add(r.window),
	})
	
	entry := val.(*struct {
		count      int
		resetTime  time.Time
		mu         sync.Mutex
	})
	
	entry.mu.Lock()
	defer entry.mu.Unlock()
	
	// 检查是否需要重置
	if time.Now().After(entry.resetTime) {
		entry.count = 0
		entry.resetTime = time.Now().Add(r.window)
	}
	
	if entry.count >= r.limit {
		return false
	}
	
	entry.count++
	return true
}

// GetBalance 获取账户余额
func (s *VulnerableTransferService) GetBalance(c *gin.Context) {
	accountID := c.Param("id")
	account, exists := s.accounts[accountID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"account": accountID,
		"balance": account.Balance,
	})
}

// GetBalance 安全获取账户余额
func (s *SafeTransferService) GetBalance(c *gin.Context) {
	accountID := c.Param("id")
	
	s.mu.RLock()
	account, exists := s.accounts[accountID]
	s.mu.RUnlock()
	
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	
	account.mu.Lock()
	balance := account.Balance
	account.mu.Unlock()
	
	c.JSON(http.StatusOK, gin.H{
		"account": accountID,
		"balance": balance,
	})
}

func main() {
	fmt.Println("=== Gin框架竞态条件漏洞演示 ===\n")
	
	// 初始化账户
	accounts := map[string]*Account{
		"alice": {ID: "alice", Balance: 1000},
		"bob":   {ID: "bob", Balance: 1000},
	}
	
	// 创建服务实例
	vulnService := &VulnerableTransferService{
		accounts: accounts,
	}
	
	safeAccounts := map[string]*Account{
		"alice": {ID: "alice", Balance: 1000},
		"bob":   {ID: "bob", Balance: 1000},
	}
	
	safeService := &SafeTransferService{
		accounts: safeAccounts,
	}
	
	// 启动存在漏洞的服务器
	go func() {
		r := gin.New()
		r.POST("/vulnerable/transfer", vulnService.VulnerableTransfer)
		r.GET("/vulnerable/balance/:id", vulnService.GetBalance)
		
		fmt.Println("[VULNERABLE] 服务器运行在 http://localhost:8082")
		r.Run(":8082")
	}()
	
	// 启动安全的服务器
	go func() {
		r := gin.New()
		r.POST("/safe/transfer", safeService.SafeTransfer)
		r.GET("/safe/balance/:id", safeService.GetBalance)
		
		fmt.Println("[SAFE] 服务器运行在 http://localhost:8083")
		r.Run(":8083")
	}()
	
	// 等待服务器启动
	time.Sleep(2 * time.Second)
	
	fmt.Println("\n=== 竞态条件测试 ===")
	fmt.Println("同时发起多个转账请求，观察余额异常:")
	fmt.Println("\n测试命令:")
	fmt.Println("1. 并发转账测试（会导致余额异常）:")
	fmt.Println("   for i in {1..10}; do")
	fmt.Println("     curl -X POST http://localhost:8082/vulnerable/transfer \\")
	fmt.Println("       -H 'Content-Type: application/json' \\")
	fmt.Println("       -d '{\"from\":\"alice\",\"to\":\"bob\",\"amount\":100}' &")
	fmt.Println("   done")
	fmt.Println("\n2. 查看余额:")
	fmt.Println("   curl http://localhost:8082/vulnerable/balance/alice")
	fmt.Println("   curl http://localhost:8082/vulnerable/balance/bob")
	fmt.Println("\n3. 安全版本测试:")
	fmt.Println("   将端口改为8083进行相同测试")
	
	// 保持主程序运行
	select {}
}