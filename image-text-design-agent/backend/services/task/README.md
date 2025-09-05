# 任务服务实现指南

## 概述

任务服务负责管理AI图文生成任务的全生命周期，包括任务创建、状态追踪、进度推送、结果存储等功能。

## 核心功能

1. **任务管理**
   - 创建生成任务
   - 查询任务状态
   - 获取任务结果
   - 任务恢复与重试

2. **实时通信**
   - WebSocket进度推送
   - SSE状态流推送
   - Redis Pub/Sub消息分发

3. **状态持久化**
   - Redis缓存热数据
   - MySQL存储完整记录
   - 支持断点恢复

## 项目结构

```
task/
├── cmd/
│   └── main.go                 # 服务入口
├── internal/
│   ├── api/                    # API层
│   │   ├── router.go          # 路由定义
│   │   ├── task_controller.go # 任务控制器
│   │   └── websocket.go       # WebSocket处理
│   ├── service/               # 业务逻辑层
│   │   ├── task_service.go    # 任务服务
│   │   ├── query_service.go   # 查询服务
│   │   └── notify_service.go  # 通知服务
│   ├── repository/            # 数据访问层
│   │   ├── task_repo.go       # 任务仓库
│   │   └── cache_repo.go      # 缓存仓库
│   ├── model/                 # 数据模型
│   │   └── task.go            # 任务模型
│   └── config/                # 配置
│       └── config.go          # 配置结构
├── pkg/                       # 公共包
│   ├── kafka/                 # Kafka客户端
│   └── redis/                 # Redis客户端
└── Dockerfile                 # 容器镜像
```

## 完整实现示例

### 1. 主程序入口

```go
// cmd/main.go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/go-redis/redis/v8"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
    
    "design-agent/task/internal/api"
    "design-agent/task/internal/config"
    "design-agent/task/internal/repository"
    "design-agent/task/internal/service"
    "design-agent/task/internal/websocket"
)

func main() {
    // 加载配置
    cfg := config.Load()
    
    // 初始化数据库
    db, err := initDB(cfg.MySQL)
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    
    // 初始化Redis
    redisClient := initRedis(cfg.Redis)
    
    // 初始化Kafka
    kafkaProducer := initKafka(cfg.Kafka)
    
    // 初始化仓库
    taskRepo := repository.NewTaskRepository(db)
    cacheRepo := repository.NewCacheRepository(redisClient)
    
    // 初始化服务
    taskService := service.NewTaskService(taskRepo, cacheRepo, kafkaProducer)
    queryService := service.NewQueryService(taskRepo, cacheRepo)
    notifyService := service.NewNotifyService(redisClient)
    
    // 初始化WebSocket Hub
    wsHub := websocket.NewHub()
    go wsHub.Run()
    
    // 订阅Redis进度更新
    go subscribeProgressUpdates(redisClient, wsHub)
    
    // 初始化控制器
    taskController := api.NewTaskController(taskService, queryService, notifyService)
    
    // 设置路由
    router := setupRouter(taskController, wsHub)
    
    // 启动服务器
    srv := &http.Server{
        Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
        Handler: router,
    }
    
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal("Failed to start server:", err)
        }
    }()
    
    log.Printf("Task service started on port %d", cfg.Server.Port)
    
    // 优雅关闭
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    log.Println("Shutting down server...")
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatal("Server forced to shutdown:", err)
    }
    
    log.Println("Server exited")
}

func setupRouter(controller *api.TaskController, hub *websocket.Hub) *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger(), gin.Recovery())
    
    // API路由
    v1 := r.Group("/api/v1")
    {
        tasks := v1.Group("/tasks")
        {
            tasks.POST("", controller.CreateTask)
            tasks.GET("/:taskId/status", controller.GetTaskStatus)
            tasks.GET("/:taskId/result", controller.GetTaskResult)
            tasks.GET("/:taskId/history", controller.GetTaskHistory)
            tasks.GET("/:taskId/stream", controller.StreamTaskProgress)
            tasks.POST("/:taskId/recover", controller.RecoverTask)
            tasks.POST("/:taskId/cancel", controller.CancelTask)
            tasks.POST("/:taskId/derivative", controller.CreateDerivativeTask)
        }
    }
    
    // WebSocket路由
    r.GET("/ws", websocket.HandleWebSocket(hub))
    
    return r
}

func subscribeProgressUpdates(redisClient *redis.Client, hub *websocket.Hub) {
    pubsub := redisClient.PSubscribe(context.Background(), "task:progress:channel:*")
    defer pubsub.Close()
    
    ch := pubsub.Channel()
    for msg := range ch {
        // 解析任务ID
        taskID := extractTaskID(msg.Channel)
        
        // 解析进度数据
        var progress map[string]interface{}
        if err := json.Unmarshal([]byte(msg.Payload), &progress); err == nil {
            hub.NotifyProgress(taskID, progress)
        }
    }
}
```

### 2. 任务服务实现

```go
// internal/service/task_service.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/google/uuid"
    "github.com/segmentio/kafka-go"
    
    "design-agent/task/internal/model"
    "design-agent/task/internal/repository"
)

type TaskService struct {
    taskRepo      repository.TaskRepository
    cacheRepo     repository.CacheRepository
    kafkaProducer *kafka.Writer
}

func NewTaskService(
    taskRepo repository.TaskRepository,
    cacheRepo repository.CacheRepository,
    kafkaProducer *kafka.Writer,
) *TaskService {
    return &TaskService{
        taskRepo:      taskRepo,
        cacheRepo:     cacheRepo,
        kafkaProducer: kafkaProducer,
    }
}

// 创建生成任务
func (s *TaskService) CreateTask(ctx context.Context, req *CreateTaskRequest) (*model.Task, error) {
    // 生成任务ID
    taskID := fmt.Sprintf("task_%s", uuid.New().String())
    
    // 创建任务对象
    task := &model.Task{
        ID:        taskID,
        UserID:    req.UserID,
        ProjectID: req.ProjectID,
        Type:      "image_generation",
        Status:    model.TaskStatusPending,
        Params:    req.Params,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    
    // 保存到数据库
    if err := s.taskRepo.Create(ctx, task); err != nil {
        return nil, fmt.Errorf("failed to create task: %w", err)
    }
    
    // 缓存任务状态
    if err := s.cacheTaskState(ctx, task); err != nil {
        // 缓存失败不影响主流程，记录日志
        log.Printf("Failed to cache task state: %v", err)
    }
    
    // 发送到Kafka队列
    message := kafka.Message{
        Key:   []byte(taskID),
        Value: s.buildTaskMessage(task),
        Headers: []kafka.Header{
            {Key: "user_id", Value: []byte(req.UserID)},
            {Key: "priority", Value: []byte(fmt.Sprintf("%d", req.Priority))},
        },
    }
    
    if err := s.kafkaProducer.WriteMessages(ctx, message); err != nil {
        // 更新任务状态为失败
        task.Status = model.TaskStatusFailed
        task.Error = &model.TaskError{
            Code:    "QUEUE_ERROR",
            Message: "Failed to enqueue task",
        }
        s.taskRepo.Update(ctx, task)
        
        return nil, fmt.Errorf("failed to enqueue task: %w", err)
    }
    
    // 更新任务状态为排队中
    task.Status = model.TaskStatusQueued
    if err := s.updateTaskStatus(ctx, task); err != nil {
        log.Printf("Failed to update task status: %v", err)
    }
    
    return task, nil
}

// 缓存任务状态
func (s *TaskService) cacheTaskState(ctx context.Context, task *model.Task) error {
    stateKey := fmt.Sprintf("task:state:%s", task.ID)
    progressKey := fmt.Sprintf("task:progress:%s", task.ID)
    
    // 缓存基本状态
    stateData := map[string]interface{}{
        "status":     task.Status,
        "phase":      "init",
        "progress":   0,
        "updated_at": task.UpdatedAt.Format(time.RFC3339),
    }
    
    if err := s.cacheRepo.HSet(ctx, stateKey, stateData, 7*24*time.Hour); err != nil {
        return err
    }
    
    // 初始化进度信息
    progressData := map[string]interface{}{
        "phase":      "init",
        "percentage": 0,
        "message":    "任务已创建，等待处理",
        "details":    "{}",
        "updated_at": time.Now().Format(time.RFC3339),
    }
    
    return s.cacheRepo.HSet(ctx, progressKey, progressData, 24*time.Hour)
}

// 更新任务状态
func (s *TaskService) updateTaskStatus(ctx context.Context, task *model.Task) error {
    // 更新数据库
    if err := s.taskRepo.Update(ctx, task); err != nil {
        return err
    }
    
    // 更新缓存
    stateKey := fmt.Sprintf("task:state:%s", task.ID)
    updates := map[string]interface{}{
        "status":     task.Status,
        "updated_at": task.UpdatedAt.Format(time.RFC3339),
    }
    
    return s.cacheRepo.HSet(ctx, stateKey, updates, 0)
}

// 取消任务
func (s *TaskService) CancelTask(ctx context.Context, taskID string) error {
    // 获取任务
    task, err := s.taskRepo.GetByID(ctx, taskID)
    if err != nil {
        return fmt.Errorf("task not found: %w", err)
    }
    
    // 检查任务状态
    if task.Status == model.TaskStatusCompleted || task.Status == model.TaskStatusFailed {
        return fmt.Errorf("cannot cancel task in status: %s", task.Status)
    }
    
    // 更新状态为已取消
    task.Status = model.TaskStatusCancelled
    task.UpdatedAt = time.Now()
    
    if err := s.updateTaskStatus(ctx, task); err != nil {
        return err
    }
    
    // 发送取消消息到Kafka
    cancelMessage := kafka.Message{
        Key: []byte(taskID),
        Value: []byte(fmt.Sprintf(`{"action":"cancel","task_id":"%s"}`, taskID)),
        Headers: []kafka.Header{
            {Key: "action", Value: []byte("cancel")},
        },
    }
    
    return s.kafkaProducer.WriteMessages(ctx, cancelMessage)
}

// 构建任务消息
func (s *TaskService) buildTaskMessage(task *model.Task) []byte {
    message := map[string]interface{}{
        "task_id":    task.ID,
        "user_id":    task.UserID,
        "project_id": task.ProjectID,
        "type":       task.Type,
        "params":     task.Params,
        "created_at": task.CreatedAt.Unix(),
    }
    
    data, _ := json.Marshal(message)
    return data
}
```

### 3. 查询服务实现

```go
// internal/service/query_service.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "design-agent/task/internal/model"
    "design-agent/task/internal/repository"
)

type QueryService struct {
    taskRepo  repository.TaskRepository
    cacheRepo repository.CacheRepository
}

func NewQueryService(
    taskRepo repository.TaskRepository,
    cacheRepo repository.CacheRepository,
) *QueryService {
    return &QueryService{
        taskRepo:  taskRepo,
        cacheRepo: cacheRepo,
    }
}

// 获取任务状态
func (s *QueryService) GetTaskStatus(ctx context.Context, taskID string) (*TaskStatus, error) {
    // 优先从缓存获取
    progressKey := fmt.Sprintf("task:progress:%s", taskID)
    progressData, err := s.cacheRepo.HGetAll(ctx, progressKey)
    
    if err == nil && len(progressData) > 0 {
        // 从缓存构建状态
        status := &TaskStatus{
            TaskID:    taskID,
            Phase:     progressData["phase"],
            Progress:  parseFloat(progressData["percentage"]),
            Message:   progressData["message"],
            UpdatedAt: parseTime(progressData["updated_at"]),
        }
        
        // 解析详情
        if details := progressData["details"]; details != "" {
            json.Unmarshal([]byte(details), &status.Details)
        }
        
        // 获取任务主状态
        stateKey := fmt.Sprintf("task:state:%s", taskID)
        if state, err := s.cacheRepo.HGet(ctx, stateKey, "status"); err == nil {
            status.Status = state
        }
        
        return status, nil
    }
    
    // 从数据库获取
    task, err := s.taskRepo.GetByID(ctx, taskID)
    if err != nil {
        return nil, fmt.Errorf("task not found: %w", err)
    }
    
    // 解析进度信息
    var progress map[string]interface{}
    if task.Progress != nil {
        json.Unmarshal([]byte(*task.Progress), &progress)
    }
    
    return &TaskStatus{
        TaskID:    task.ID,
        Status:    task.Status,
        Phase:     getStringValue(progress, "phase", "init"),
        Progress:  getFloatValue(progress, "percentage", 0),
        Message:   getStringValue(progress, "message", ""),
        UpdatedAt: task.UpdatedAt,
    }, nil
}

// 获取任务结果
func (s *QueryService) GetTaskResult(ctx context.Context, taskID string) (*model.GenerationResult, error) {
    // 优先从缓存获取
    resultKey := fmt.Sprintf("task:result:%s", taskID)
    if cached, err := s.cacheRepo.Get(ctx, resultKey); err == nil && cached != "" {
        var result model.GenerationResult
        if err := json.Unmarshal([]byte(cached), &result); err == nil {
            return &result, nil
        }
    }
    
    // 从数据库获取
    task, err := s.taskRepo.GetByID(ctx, taskID)
    if err != nil {
        return nil, fmt.Errorf("task not found: %w", err)
    }
    
    if task.Result == nil {
        return nil, fmt.Errorf("task result not available")
    }
    
    var result model.GenerationResult
    if err := json.Unmarshal([]byte(*task.Result), &result); err != nil {
        return nil, fmt.Errorf("failed to parse result: %w", err)
    }
    
    // 缓存结果
    if data, err := json.Marshal(result); err == nil {
        s.cacheRepo.Set(ctx, resultKey, string(data), 3*24*time.Hour)
    }
    
    return &result, nil
}

// 获取任务历史
func (s *QueryService) GetTaskHistory(ctx context.Context, taskID string) ([]TaskProgress, error) {
    logs, err := s.taskRepo.GetProgressLogs(ctx, taskID)
    if err != nil {
        return nil, err
    }
    
    history := make([]TaskProgress, len(logs))
    for i, log := range logs {
        history[i] = TaskProgress{
            Phase:     log.Phase,
            Progress:  log.Progress,
            Message:   log.Message,
            Timestamp: log.CreatedAt,
        }
    }
    
    return history, nil
}

// 获取任务快照（用于调试）
func (s *QueryService) GetTaskSnapshot(ctx context.Context, taskID string, version string) (*TaskSnapshot, error) {
    var snapshot *model.TaskSnapshot
    var err error
    
    if version == "latest" {
        snapshot, err = s.taskRepo.GetLatestSnapshot(ctx, taskID)
    } else {
        snapshot, err = s.taskRepo.GetSnapshot(ctx, taskID, version)
    }
    
    if err != nil {
        return nil, fmt.Errorf("snapshot not found: %w", err)
    }
    
    // 解析状态数据
    var stateData map[string]interface{}
    if err := json.Unmarshal([]byte(snapshot.StateData), &stateData); err != nil {
        return nil, fmt.Errorf("failed to parse state data: %w", err)
    }
    
    return &TaskSnapshot{
        TaskID:    snapshot.TaskID,
        Version:   snapshot.Version,
        State:     stateData,
        CreatedAt: snapshot.CreatedAt,
    }, nil
}

// 订阅任务进度
func (s *QueryService) SubscribeTaskProgress(
    ctx context.Context,
    taskID string,
    callback func(TaskStatus),
) error {
    // 创建Redis订阅
    channel := fmt.Sprintf("task:progress:channel:%s", taskID)
    pubsub := s.cacheRepo.Subscribe(ctx, channel)
    defer pubsub.Close()
    
    // 获取初始状态
    if status, err := s.GetTaskStatus(ctx, taskID); err == nil {
        callback(*status)
    }
    
    // 监听更新
    ch := pubsub.Channel()
    for {
        select {
        case msg := <-ch:
            var progress map[string]interface{}
            if err := json.Unmarshal([]byte(msg.Payload), &progress); err == nil {
                status := TaskStatus{
                    TaskID:    taskID,
                    Phase:     getStringValue(progress, "phase", ""),
                    Progress:  getFloatValue(progress, "percentage", 0),
                    Message:   getStringValue(progress, "message", ""),
                    Details:   progress,
                    UpdatedAt: time.Now(),
                }
                callback(status)
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 4. WebSocket实现

```go
// internal/websocket/client.go
package websocket

import (
    "log"
    "time"
    
    "github.com/gorilla/websocket"
)

const (
    writeWait      = 10 * time.Second
    pongWait       = 60 * time.Second
    pingPeriod     = (pongWait * 9) / 10
    maxMessageSize = 512
)

type Client struct {
    hub    *Hub
    conn   *websocket.Conn
    send   chan []byte
    taskID string
    userID string
}

func (c *Client) readPump() {
    defer func() {
        c.hub.unregister <- c
        c.conn.Close()
    }()
    
    c.conn.SetReadLimit(maxMessageSize)
    c.conn.SetReadDeadline(time.Now().Add(pongWait))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })
    
    for {
        _, message, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("websocket error: %v", err)
            }
            break
        }
        
        // 处理客户端消息（如心跳）
        log.Printf("Received message from client: %s", message)
    }
}

func (c *Client) writePump() {
    ticker := time.NewTicker(pingPeriod)
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()
    
    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if !ok {
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            
            w, err := c.conn.NextWriter(websocket.TextMessage)
            if err != nil {
                return
            }
            w.Write(message)
            
            // 批量发送队列中的消息
            n := len(c.send)
            for i := 0; i < n; i++ {
                w.Write([]byte("\n"))
                w.Write(<-c.send)
            }
            
            if err := w.Close(); err != nil {
                return
            }
            
        case <-ticker.C:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}
```

### 5. 路由定义

```go
// internal/api/router.go
package api

import (
    "github.com/gin-gonic/gin"
    
    "design-agent/pkg/middleware"
)

func SetupRoutes(r *gin.Engine, controller *TaskController, wsHub *websocket.Hub) {
    // 中间件
    r.Use(middleware.RequestID())
    r.Use(middleware.Logger())
    r.Use(middleware.ErrorHandler())
    r.Use(middleware.CORS())
    
    // 健康检查
    r.GET("/health", func(c *gin.Context) {
        c.JSON(200, gin.H{"status": "ok"})
    })
    
    // API版本
    v1 := r.Group("/api/v1")
    v1.Use(middleware.Auth()) // JWT认证
    
    // 任务相关接口
    tasks := v1.Group("/tasks")
    {
        // 创建任务
        tasks.POST("", middleware.RateLimit("create", 10), controller.CreateTask)
        
        // 查询接口
        tasks.GET("/:taskId/status", controller.GetTaskStatus)
        tasks.GET("/:taskId/result", controller.GetTaskResult)
        tasks.GET("/:taskId/history", controller.GetTaskHistory)
        tasks.GET("/:taskId/snapshot", controller.GetTaskSnapshot)
        
        // 实时推送
        tasks.GET("/:taskId/stream", controller.StreamTaskProgress)
        
        // 控制接口
        tasks.POST("/:taskId/cancel", controller.CancelTask)
        tasks.POST("/:taskId/recover", controller.RecoverTask)
        tasks.POST("/:taskId/derivative", controller.CreateDerivativeTask)
    }
    
    // WebSocket（需要单独的认证）
    r.GET("/ws", middleware.WSAuth(), websocket.HandleWebSocket(wsHub))
}
```

## 部署配置

### Docker配置

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o task-service cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/task-service .
COPY --from=builder /app/configs ./configs

EXPOSE 8080
CMD ["./task-service"]
```

### Kubernetes配置

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: task-service
  namespace: design-agent-prod
spec:
  replicas: 3
  selector:
    matchLabels:
      app: task-service
  template:
    metadata:
      labels:
        app: task-service
    spec:
      containers:
      - name: task-service
        image: registry.example.com/design-agent/task-service:latest
        ports:
        - containerPort: 8080
        env:
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: host
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: redis-secret
              key: url
        - name: KAFKA_BROKERS
          value: "kafka-0:9092,kafka-1:9092,kafka-2:9092"
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
```

## 性能优化建议

1. **连接池优化**
   - MySQL连接池：最大100，空闲10
   - Redis连接池：最大200，空闲20

2. **缓存策略**
   - 任务状态缓存7天
   - 进度信息缓存24小时
   - 结果数据缓存3天

3. **批量处理**
   - WebSocket消息批量发送
   - 数据库批量更新

4. **限流保护**
   - 创建任务：10次/分钟
   - 状态查询：100次/分钟
   - WebSocket连接：1000个/节点

## 监控指标

- 任务创建速率
- 任务完成速率
- 平均处理时间
- WebSocket连接数
- Redis命中率
- 错误率