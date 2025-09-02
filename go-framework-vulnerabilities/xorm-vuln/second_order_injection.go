package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"xorm.io/xorm"
)

// User 用户模型
type User struct {
	ID       int64     `xorm:"pk autoincr"`
	Username string    `xorm:"unique not null"`
	Email    string    `xorm:"not null"`
	Profile  string    `xorm:"text"` // 用户简介，可能包含恶意内容
	Created  time.Time `xorm:"created"`
}

// Comment 评论模型
type Comment struct {
	ID        int64     `xorm:"pk autoincr"`
	UserID    int64     `xorm:"not null"`
	Content   string    `xorm:"text"`
	CreatedAt time.Time `xorm:"created"`
}

// SearchLog 搜索日志模型
type SearchLog struct {
	ID        int64     `xorm:"pk autoincr"`
	UserID    int64     `xorm:"not null"`
	Query     string    `xorm:"not null"` // 存储用户的搜索查询
	Results   int       `xorm:"not null"`
	CreatedAt time.Time `xorm:"created"`
}

// VulnerableSecondOrderInjection 演示二阶SQL注入漏洞
type VulnerableSecondOrderInjection struct {
	engine *xorm.Engine
}

// CreateUser 创建用户 - 第一阶段：存储恶意数据
func (v *VulnerableSecondOrderInjection) CreateUser(username, email, profile string) error {
	user := &User{
		Username: username,
		Email:    email,
		Profile:  profile, // 这里可能包含恶意SQL代码
	}
	
	// 使用参数化查询插入数据（这部分是安全的）
	_, err := v.engine.Insert(user)
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}
	
	fmt.Printf("用户创建成功: %s (Profile: %s)\n", username, profile)
	return nil
}

// VulnerableSearchUsersByProfile 漏洞函数 - 第二阶段：执行恶意代码
func (v *VulnerableSecondOrderInjection) VulnerableSearchUsersByProfile(keyword string) error {
	var users []User
	
	// 首先获取所有用户
	err := v.engine.Find(&users)
	if err != nil {
		return err
	}
	
	// 漏洞点：使用存储的用户数据构建动态SQL
	for _, user := range users {
		// 危险：直接使用数据库中存储的数据构建SQL查询
		query := fmt.Sprintf("SELECT * FROM comment WHERE content LIKE '%%%s%%' OR content LIKE '%%%s%%'", 
			keyword, user.Profile)
		
		var comments []Comment
		err := v.engine.SQL(query).Find(&comments)
		if err != nil {
			// SQL注入可能在这里触发
			fmt.Printf("查询出错 (可能是SQL注入): %v\n", err)
			fmt.Printf("问题查询: %s\n", query)
		} else {
			fmt.Printf("用户 %s: 找到 %d 条相关评论\n", user.Username, len(comments))
		}
	}
	
	return nil
}

// VulnerableLogSearch 另一个二阶注入示例
func (v *VulnerableSecondOrderInjection) VulnerableLogSearch(userID int64, searchQuery string) error {
	// 第一步：安全地存储搜索查询
	log := &SearchLog{
		UserID:  userID,
		Query:   searchQuery, // 可能包含恶意SQL
		Results: 0,
	}
	
	_, err := v.engine.Insert(log)
	if err != nil {
		return err
	}
	
	// 第二步：在后续的统计分析中使用存储的查询
	// 漏洞：管理员查看搜索统计时
	var logs []SearchLog
	err = v.engine.Where("user_id = ?", userID).Find(&logs)
	if err != nil {
		return err
	}
	
	// 危险：使用存储的搜索查询构建新的SQL
	for _, log := range logs {
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM comment WHERE content LIKE '%%%s%%'", log.Query)
		
		var count int64
		_, err := v.engine.SQL(countQuery).Get(&count)
		if err != nil {
			fmt.Printf("统计查询失败 (可能是二阶SQL注入): %v\n", err)
			fmt.Printf("问题查询: %s\n", countQuery)
		}
	}
	
	return nil
}

// SafeSearchUsersByProfile 安全的搜索实现
func (v *VulnerableSecondOrderInjection) SafeSearchUsersByProfile(keyword string) error {
	var users []User
	
	err := v.engine.Find(&users)
	if err != nil {
		return err
	}
	
	for _, user := range users {
		// 安全：使用参数化查询
		var comments []Comment
		err := v.engine.Where("content LIKE ? OR content LIKE ?", 
			"%"+keyword+"%", "%"+user.Profile+"%").Find(&comments)
		if err != nil {
			fmt.Printf("查询出错: %v\n", err)
		} else {
			fmt.Printf("用户 %s: 找到 %d 条相关评论\n", user.Username, len(comments))
		}
	}
	
	return nil
}

// SafeLogSearch 安全的搜索日志实现
func (v *VulnerableSecondOrderInjection) SafeLogSearch(userID int64, searchQuery string) error {
	// 存储搜索查询（同样的方式）
	log := &SearchLog{
		UserID:  userID,
		Query:   searchQuery,
		Results: 0,
	}
	
	_, err := v.engine.Insert(log)
	if err != nil {
		return err
	}
	
	// 安全地使用存储的查询
	var logs []SearchLog
	err = v.engine.Where("user_id = ?", userID).Find(&logs)
	if err != nil {
		return err
	}
	
	for _, log := range logs {
		// 使用参数化查询而不是字符串拼接
		var count int64
		count, err := v.engine.Where("content LIKE ?", "%"+log.Query+"%").Count(&Comment{})
		if err != nil {
			fmt.Printf("统计查询失败: %v\n", err)
		} else {
			fmt.Printf("查询 '%s' 匹配了 %d 条评论\n", log.Query, count)
		}
	}
	
	return nil
}

// InitDatabase 初始化数据库
func InitDatabase() (*xorm.Engine, error) {
	engine, err := xorm.NewEngine("sqlite3", "./test.db")
	if err != nil {
		return nil, err
	}
	
	// 创建表
	err = engine.Sync2(new(User), new(Comment), new(SearchLog))
	if err != nil {
		return nil, err
	}
	
	// 插入一些测试数据
	comments := []Comment{
		{UserID: 1, Content: "这是一条普通评论"},
		{UserID: 1, Content: "另一条测试评论"},
		{UserID: 2, Content: "管理员的评论"},
	}
	
	for _, comment := range comments {
		_, err = engine.Insert(&comment)
		if err != nil {
			return nil, err
		}
	}
	
	return engine, nil
}

func main() {
	fmt.Println("=== XORM二阶SQL注入漏洞演示 ===\n")
	
	// 初始化数据库
	engine, err := InitDatabase()
	if err != nil {
		log.Fatal("数据库初始化失败:", err)
	}
	defer engine.Close()
	
	vuln := &VulnerableSecondOrderInjection{engine: engine}
	
	fmt.Println("1. 创建正常用户:")
	vuln.CreateUser("normal_user", "normal@example.com", "我是一个普通用户")
	
	fmt.Println("\n2. 创建包含恶意SQL的用户:")
	// 恶意profile包含SQL注入代码
	maliciousProfile := "'; DROP TABLE comment; --"
	vuln.CreateUser("evil_user", "evil@example.com", maliciousProfile)
	
	fmt.Println("\n3. 执行漏洞搜索（二阶SQL注入）:")
	fmt.Println("当搜索功能使用存储的用户数据时...")
	err = vuln.VulnerableSearchUsersByProfile("test")
	if err != nil {
		fmt.Printf("搜索失败: %v\n", err)
	}
	
	fmt.Println("\n4. 演示搜索日志的二阶注入:")
	maliciousSearch := "' OR '1'='1"
	vuln.VulnerableLogSearch(1, maliciousSearch)
	
	fmt.Println("\n5. 使用安全的方法:")
	fmt.Println("安全搜索用户资料:")
	vuln.SafeSearchUsersByProfile("test")
	
	fmt.Println("\n安全搜索日志:")
	vuln.SafeLogSearch(1, maliciousSearch)
	
	fmt.Println("\n=== 漏洞分析 ===")
	fmt.Println("二阶SQL注入的特点:")
	fmt.Println("1. 恶意数据首先被安全地存储到数据库中")
	fmt.Println("2. 在后续的操作中，这些数据被取出并不安全地使用")
	fmt.Println("3. 攻击者可以通过用户资料、评论等字段注入恶意SQL")
	fmt.Println("4. 防御方法：始终使用参数化查询，即使处理来自数据库的数据")
}