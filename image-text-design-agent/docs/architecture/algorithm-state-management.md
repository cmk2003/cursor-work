# 算法端状态管理与服务端接口设计

## 1. 状态存储架构设计

### 1.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        前端应用                              │
│                    (查询进度/结果)                           │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     服务端任务服务                           │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Task API    │  │ Status Query │  │ Result Query │      │
│  │ Controller  │  │   Service    │  │   Service    │      │
│  └─────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
┌─────────────────────────┐  ┌─────────────────────────┐
│      Redis缓存          │  │     MySQL数据库         │
│  - 实时状态             │  │  - 任务记录            │
│  - 进度信息             │  │  - 历史结果            │
│  - 临时数据             │  │  - 参数配置            │
└─────────────────────────┘  └─────────────────────────┘
                    ▲                   ▲
                    └─────────┬─────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      算法端服务                              │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ State       │  │ Progress     │  │ Result       │      │
│  │ Manager     │  │ Reporter     │  │ Persister    │      │
│  └─────────────┘  └──────────────┘  └──────────────┘      │
│  ┌────────────────────────────────────────────────────┐    │
│  │           LangGraph Workflow Engine                 │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 状态数据模型

```python
# algorithm/models/state_models.py
from typing import Dict, List, Any, Optional
from datetime import datetime
from pydantic import BaseModel
from enum import Enum

class TaskStatus(str, Enum):
    PENDING = "pending"
    QUEUED = "queued"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"

class TaskPhase(str, Enum):
    INIT = "init"
    PARSING_INTENT = "parsing_intent"
    GENERATING_ELEMENTS = "generating_elements"
    GENERATING_IMAGE = "generating_image"
    CHECKING_OCR = "checking_ocr"
    ASSESSING_QUALITY = "assessing_quality"
    REFINING = "refining"
    FINALIZING = "finalizing"

class TaskState(BaseModel):
    """任务状态模型"""
    task_id: str
    user_id: str
    project_id: Optional[str]
    status: TaskStatus
    phase: TaskPhase
    progress: float  # 0-100
    
    # 输入参数
    input_params: Dict[str, Any]
    
    # 中间状态
    intermediate_states: Dict[str, Any] = {}
    
    # 阶段结果
    phase_results: Dict[str, Any] = {}
    
    # 错误信息
    error: Optional[Dict[str, Any]]
    
    # 时间信息
    created_at: datetime
    started_at: Optional[datetime]
    updated_at: datetime
    completed_at: Optional[datetime]
    
    # 版本信息（用于回溯）
    version: int = 1
    parent_task_id: Optional[str]  # 用于重试/衍生任务

class GenerationResult(BaseModel):
    """生成结果模型"""
    task_id: str
    design_id: str
    
    # 生成的图片
    images: List[Dict[str, Any]]  # URL, metadata等
    
    # 可编辑元素
    editable_elements: List[Dict[str, Any]]
    
    # 质量评分
    quality_scores: Dict[str, float]
    
    # 生成参数（用于复现）
    generation_params: Dict[str, Any]
    
    # 模型信息
    model_info: Dict[str, str]
    
    # 性能指标
    metrics: Dict[str, Any]
```

## 2. 算法端状态管理实现

### 2.1 状态管理器

```python
# algorithm/core/state_manager.py
import json
import pickle
from typing import Any, Dict, Optional
from datetime import datetime, timedelta
import asyncio
from redis import asyncio as aioredis
import aiomysql
from contextlib import asynccontextmanager

class StateManager:
    """算法端状态管理器"""
    
    def __init__(self, redis_url: str, mysql_config: Dict):
        self.redis_url = redis_url
        self.mysql_config = mysql_config
        self._redis_pool = None
        self._mysql_pool = None
    
    async def init(self):
        """初始化连接池"""
        self._redis_pool = await aioredis.create_redis_pool(self.redis_url)
        self._mysql_pool = await aiomysql.create_pool(**self.mysql_config)
    
    async def close(self):
        """关闭连接池"""
        if self._redis_pool:
            self._redis_pool.close()
            await self._redis_pool.wait_closed()
        if self._mysql_pool:
            self._mysql_pool.close()
            await self._mysql_pool.wait_closed()
    
    @asynccontextmanager
    async def redis(self):
        """Redis连接上下文管理器"""
        async with self._redis_pool.get() as conn:
            yield conn
    
    @asynccontextmanager
    async def mysql(self):
        """MySQL连接上下文管理器"""
        async with self._mysql_pool.acquire() as conn:
            async with conn.cursor(aiomysql.DictCursor) as cursor:
                yield cursor
            await conn.commit()
    
    async def save_task_state(self, task_id: str, state: TaskState):
        """保存任务状态"""
        # 1. 保存到Redis（用于实时查询）
        async with self.redis() as redis:
            # 主状态
            await redis.hset(
                f"task:state:{task_id}",
                mapping={
                    "status": state.status,
                    "phase": state.phase,
                    "progress": state.progress,
                    "updated_at": state.updated_at.isoformat()
                }
            )
            
            # 设置过期时间（7天）
            await redis.expire(f"task:state:{task_id}", 7 * 24 * 3600)
            
            # 中间状态（用于断点恢复）
            if state.intermediate_states:
                await redis.set(
                    f"task:intermediate:{task_id}",
                    pickle.dumps(state.intermediate_states),
                    expire=24 * 3600  # 24小时
                )
        
        # 2. 异步保存到MySQL（用于持久化）
        asyncio.create_task(self._persist_to_mysql(task_id, state))
    
    async def _persist_to_mysql(self, task_id: str, state: TaskState):
        """持久化到MySQL"""
        async with self.mysql() as cursor:
            # 更新任务主表
            await cursor.execute("""
                UPDATE generation_tasks 
                SET status = %s, 
                    progress = %s,
                    current_phase = %s,
                    updated_at = %s
                WHERE id = %s
            """, (
                state.status,
                json.dumps({"phase": state.phase, "percentage": state.progress}),
                state.phase,
                state.updated_at,
                task_id
            ))
            
            # 保存状态快照（用于回溯）
            await cursor.execute("""
                INSERT INTO task_state_snapshots 
                (task_id, version, state_data, created_at)
                VALUES (%s, %s, %s, %s)
            """, (
                task_id,
                state.version,
                json.dumps(state.dict()),
                datetime.now()
            ))
    
    async def get_task_state(self, task_id: str) -> Optional[TaskState]:
        """获取任务状态"""
        # 优先从Redis获取
        async with self.redis() as redis:
            state_data = await redis.hgetall(f"task:state:{task_id}")
            
            if state_data:
                # 获取中间状态
                intermediate_data = await redis.get(f"task:intermediate:{task_id}")
                intermediate_states = pickle.loads(intermediate_data) if intermediate_data else {}
                
                return TaskState(
                    task_id=task_id,
                    status=state_data[b'status'].decode(),
                    phase=state_data[b'phase'].decode(),
                    progress=float(state_data[b'progress']),
                    intermediate_states=intermediate_states,
                    updated_at=datetime.fromisoformat(state_data[b'updated_at'].decode())
                )
        
        # Redis没有则从MySQL获取
        async with self.mysql() as cursor:
            await cursor.execute("""
                SELECT * FROM generation_tasks WHERE id = %s
            """, (task_id,))
            
            row = await cursor.fetchone()
            if row:
                return self._mysql_row_to_task_state(row)
        
        return None
    
    async def save_checkpoint(self, task_id: str, phase: str, data: Dict[str, Any]):
        """保存检查点（用于断点恢复）"""
        checkpoint = {
            "phase": phase,
            "data": data,
            "timestamp": datetime.now().isoformat()
        }
        
        async with self.redis() as redis:
            # 保存检查点
            await redis.rpush(
                f"task:checkpoints:{task_id}",
                json.dumps(checkpoint)
            )
            
            # 只保留最近10个检查点
            await redis.ltrim(f"task:checkpoints:{task_id}", -10, -1)
    
    async def get_latest_checkpoint(self, task_id: str) -> Optional[Dict[str, Any]]:
        """获取最新检查点"""
        async with self.redis() as redis:
            checkpoint_data = await redis.lindex(f"task:checkpoints:{task_id}", -1)
            
            if checkpoint_data:
                return json.loads(checkpoint_data)
        
        return None
    
    async def save_result(self, task_id: str, result: GenerationResult):
        """保存生成结果"""
        # 1. 保存到Redis（用于快速访问）
        async with self.redis() as redis:
            await redis.set(
                f"task:result:{task_id}",
                json.dumps(result.dict()),
                expire=3 * 24 * 3600  # 3天
            )
        
        # 2. 持久化到MySQL
        async with self.mysql() as cursor:
            await cursor.execute("""
                UPDATE generation_tasks 
                SET result = %s,
                    completed_at = %s,
                    status = 'completed'
                WHERE id = %s
            """, (
                json.dumps(result.dict()),
                datetime.now(),
                task_id
            ))
            
            # 保存到设计表
            await cursor.execute("""
                INSERT INTO designs 
                (id, project_id, user_id, task_id, canvas_data, preview_url, created_at)
                VALUES (%s, %s, %s, %s, %s, %s, %s)
            """, (
                result.design_id,
                result.project_id,
                result.user_id,
                task_id,
                json.dumps(result.editable_elements),
                result.images[0]['url'] if result.images else None,
                datetime.now()
            ))
```

### 2.2 进度上报器

```python
# algorithm/core/progress_reporter.py
from typing import Dict, Any, Optional, Callable
import asyncio
from datetime import datetime

class ProgressReporter:
    """进度上报器"""
    
    def __init__(self, state_manager: StateManager, websocket_notifier: Optional[Callable] = None):
        self.state_manager = state_manager
        self.websocket_notifier = websocket_notifier
        self._progress_cache = {}
        self._report_interval = 0.5  # 最小上报间隔（秒）
        self._last_report_time = {}
    
    async def report_progress(
        self,
        task_id: str,
        phase: TaskPhase,
        progress: float,
        message: str = "",
        details: Optional[Dict[str, Any]] = None
    ):
        """上报进度"""
        current_time = datetime.now()
        
        # 限流：避免过于频繁的更新
        last_time = self._last_report_time.get(task_id)
        if last_time and (current_time - last_time).total_seconds() < self._report_interval:
            # 缓存起来，稍后批量更新
            self._progress_cache[task_id] = {
                "phase": phase,
                "progress": progress,
                "message": message,
                "details": details,
                "timestamp": current_time
            }
            return
        
        # 更新进度
        await self._do_report(task_id, phase, progress, message, details)
        self._last_report_time[task_id] = current_time
    
    async def _do_report(
        self,
        task_id: str,
        phase: TaskPhase,
        progress: float,
        message: str,
        details: Optional[Dict[str, Any]]
    ):
        """实际执行上报"""
        # 1. 更新Redis状态
        async with self.state_manager.redis() as redis:
            progress_data = {
                "phase": phase.value,
                "percentage": progress,
                "message": message,
                "details": json.dumps(details) if details else "{}",
                "updated_at": datetime.now().isoformat()
            }
            
            await redis.hset(
                f"task:progress:{task_id}",
                mapping=progress_data
            )
            
            # 发布进度事件
            await redis.publish(
                f"task:progress:channel:{task_id}",
                json.dumps(progress_data)
            )
        
        # 2. 通过WebSocket推送（如果配置了）
        if self.websocket_notifier:
            await self.websocket_notifier(task_id, {
                "type": "progress",
                "data": progress_data
            })
        
        # 3. 记录关键节点到MySQL（异步）
        if progress % 10 == 0 or phase in [TaskPhase.INIT, TaskPhase.FINALIZING]:
            asyncio.create_task(self._log_progress_to_db(task_id, phase, progress, message))
    
    async def _log_progress_to_db(self, task_id: str, phase: TaskPhase, progress: float, message: str):
        """记录进度到数据库"""
        async with self.state_manager.mysql() as cursor:
            await cursor.execute("""
                INSERT INTO task_progress_logs 
                (task_id, phase, progress, message, created_at)
                VALUES (%s, %s, %s, %s, %s)
            """, (task_id, phase.value, progress, message, datetime.now()))
    
    async def report_phase_complete(
        self,
        task_id: str,
        phase: TaskPhase,
        result: Dict[str, Any]
    ):
        """上报阶段完成"""
        # 保存阶段结果
        async with self.state_manager.redis() as redis:
            await redis.hset(
                f"task:phase_results:{task_id}",
                phase.value,
                json.dumps(result)
            )
        
        # 计算总进度
        phase_weights = {
            TaskPhase.PARSING_INTENT: 10,
            TaskPhase.GENERATING_ELEMENTS: 20,
            TaskPhase.GENERATING_IMAGE: 50,
            TaskPhase.CHECKING_OCR: 70,
            TaskPhase.ASSESSING_QUALITY: 85,
            TaskPhase.REFINING: 95,
            TaskPhase.FINALIZING: 100
        }
        
        progress = phase_weights.get(phase, 0)
        await self.report_progress(
            task_id,
            phase,
            progress,
            f"{phase.value} completed",
            {"result_summary": self._summarize_result(result)}
        )
    
    def _summarize_result(self, result: Dict[str, Any]) -> Dict[str, Any]:
        """总结阶段结果（避免存储过大数据）"""
        summary = {}
        
        for key, value in result.items():
            if isinstance(value, (str, int, float, bool)):
                summary[key] = value
            elif isinstance(value, list):
                summary[f"{key}_count"] = len(value)
            elif isinstance(value, dict):
                summary[f"{key}_keys"] = list(value.keys())[:5]
        
        return summary
```

### 2.3 任务恢复管理器

```python
# algorithm/core/recovery_manager.py
class RecoveryManager:
    """任务恢复管理器"""
    
    def __init__(self, state_manager: StateManager):
        self.state_manager = state_manager
    
    async def can_recover(self, task_id: str) -> bool:
        """检查任务是否可恢复"""
        state = await self.state_manager.get_task_state(task_id)
        
        if not state:
            return False
        
        # 只有处理中或失败的任务可以恢复
        return state.status in [TaskStatus.PROCESSING, TaskStatus.FAILED]
    
    async def recover_task(self, task_id: str) -> Optional[Dict[str, Any]]:
        """恢复任务到最近的检查点"""
        # 1. 获取最新检查点
        checkpoint = await self.state_manager.get_latest_checkpoint(task_id)
        
        if not checkpoint:
            return None
        
        # 2. 获取任务状态
        state = await self.state_manager.get_task_state(task_id)
        
        if not state:
            return None
        
        # 3. 构建恢复数据
        recovery_data = {
            "task_id": task_id,
            "resume_from_phase": checkpoint["phase"],
            "checkpoint_data": checkpoint["data"],
            "original_params": state.input_params,
            "intermediate_states": state.intermediate_states,
            "phase_results": state.phase_results
        }
        
        # 4. 更新任务状态
        state.status = TaskStatus.PROCESSING
        state.version += 1
        await self.state_manager.save_task_state(task_id, state)
        
        return recovery_data
    
    async def create_derivative_task(
        self,
        parent_task_id: str,
        modifications: Dict[str, Any]
    ) -> str:
        """基于已有任务创建衍生任务"""
        # 1. 获取父任务信息
        parent_state = await self.state_manager.get_task_state(parent_task_id)
        
        if not parent_state:
            raise ValueError(f"Parent task {parent_task_id} not found")
        
        # 2. 创建新任务
        new_task_id = f"task_{uuid.uuid4().hex}"
        
        # 3. 合并参数
        new_params = {**parent_state.input_params, **modifications}
        
        # 4. 创建任务状态
        new_state = TaskState(
            task_id=new_task_id,
            user_id=parent_state.user_id,
            project_id=parent_state.project_id,
            status=TaskStatus.PENDING,
            phase=TaskPhase.INIT,
            progress=0,
            input_params=new_params,
            created_at=datetime.now(),
            updated_at=datetime.now(),
            parent_task_id=parent_task_id
        )
        
        # 5. 保存任务
        await self.state_manager.save_task_state(new_task_id, new_state)
        
        return new_task_id
```

## 3. 服务端接口实现

### 3.1 任务查询服务（Go）

```go
// backend/services/task/internal/service/query_service.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/go-redis/redis/v8"
    "gorm.io/gorm"
)

type TaskQueryService struct {
    db    *gorm.DB
    redis *redis.Client
}

// 任务状态结构
type TaskStatus struct {
    TaskID      string                 `json:"task_id"`
    Status      string                 `json:"status"`
    Phase       string                 `json:"phase"`
    Progress    float64                `json:"progress"`
    Message     string                 `json:"message"`
    Details     map[string]interface{} `json:"details"`
    UpdatedAt   time.Time              `json:"updated_at"`
}

// 获取任务状态
func (s *TaskQueryService) GetTaskStatus(ctx context.Context, taskID string) (*TaskStatus, error) {
    // 1. 先从Redis获取实时状态
    progressKey := fmt.Sprintf("task:progress:%s", taskID)
    progressData, err := s.redis.HGetAll(ctx, progressKey).Result()
    
    if err == nil && len(progressData) > 0 {
        // 解析Redis数据
        status := &TaskStatus{
            TaskID:   taskID,
            Phase:    progressData["phase"],
            Message:  progressData["message"],
        }
        
        // 解析进度
        if progress, err := strconv.ParseFloat(progressData["percentage"], 64); err == nil {
            status.Progress = progress
        }
        
        // 解析详情
        if details := progressData["details"]; details != "" {
            json.Unmarshal([]byte(details), &status.Details)
        }
        
        // 解析时间
        if updatedAt, err := time.Parse(time.RFC3339, progressData["updated_at"]); err == nil {
            status.UpdatedAt = updatedAt
        }
        
        // 获取任务主状态
        stateKey := fmt.Sprintf("task:state:%s", taskID)
        if stateData, err := s.redis.HGet(ctx, stateKey, "status").Result(); err == nil {
            status.Status = stateData
        }
        
        return status, nil
    }
    
    // 2. Redis没有则从数据库获取
    var task models.GenerationTask
    if err := s.db.Where("id = ?", taskID).First(&task).Error; err != nil {
        return nil, err
    }
    
    // 解析进度信息
    var progressInfo map[string]interface{}
    if task.Progress != nil {
        json.Unmarshal([]byte(*task.Progress), &progressInfo)
    }
    
    return &TaskStatus{
        TaskID:    task.ID,
        Status:    task.Status,
        Phase:     task.CurrentPhase,
        Progress:  progressInfo["percentage"].(float64),
        Message:   progressInfo["message"].(string),
        UpdatedAt: task.UpdatedAt,
    }, nil
}

// 获取任务历史
func (s *TaskQueryService) GetTaskHistory(ctx context.Context, taskID string) ([]TaskProgress, error) {
    var logs []models.TaskProgressLog
    
    err := s.db.Where("task_id = ?", taskID).
        Order("created_at ASC").
        Find(&logs).Error
    
    if err != nil {
        return nil, err
    }
    
    // 转换为返回格式
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

// 获取任务结果
func (s *TaskQueryService) GetTaskResult(ctx context.Context, taskID string) (*GenerationResult, error) {
    // 1. 先从Redis获取
    resultKey := fmt.Sprintf("task:result:%s", taskID)
    resultData, err := s.redis.Get(ctx, resultKey).Result()
    
    if err == nil && resultData != "" {
        var result GenerationResult
        if err := json.Unmarshal([]byte(resultData), &result); err == nil {
            return &result, nil
        }
    }
    
    // 2. 从数据库获取
    var task models.GenerationTask
    if err := s.db.Where("id = ?", taskID).First(&task).Error; err != nil {
        return nil, err
    }
    
    if task.Result == nil {
        return nil, fmt.Errorf("task result not found")
    }
    
    var result GenerationResult
    if err := json.Unmarshal([]byte(*task.Result), &result); err != nil {
        return nil, err
    }
    
    return &result, nil
}

// 订阅任务进度（用于SSE或WebSocket）
func (s *TaskQueryService) SubscribeTaskProgress(ctx context.Context, taskID string, callback func(TaskStatus)) error {
    pubsub := s.redis.Subscribe(ctx, fmt.Sprintf("task:progress:channel:%s", taskID))
    defer pubsub.Close()
    
    ch := pubsub.Channel()
    
    for {
        select {
        case msg := <-ch:
            var progress TaskStatus
            if err := json.Unmarshal([]byte(msg.Payload), &progress); err == nil {
                progress.TaskID = taskID
                callback(progress)
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 3.2 任务控制API（Go）

```go
// backend/services/task/internal/api/task_controller.go
package api

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
)

type TaskController struct {
    queryService   *service.TaskQueryService
    controlService *service.TaskControlService
}

// 查询任务状态
func (c *TaskController) GetTaskStatus(ctx *gin.Context) {
    taskID := ctx.Param("taskId")
    
    status, err := c.queryService.GetTaskStatus(ctx, taskID)
    if err != nil {
        ctx.JSON(http.StatusNotFound, gin.H{
            "code": 404,
            "message": "Task not found",
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 0,
        "data": status,
    })
}

// 获取任务进度历史
func (c *TaskController) GetTaskHistory(ctx *gin.Context) {
    taskID := ctx.Param("taskId")
    
    history, err := c.queryService.GetTaskHistory(ctx, taskID)
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code": 500,
            "message": "Failed to get task history",
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 0,
        "data": history,
    })
}

// 获取任务结果
func (c *TaskController) GetTaskResult(ctx *gin.Context) {
    taskID := ctx.Param("taskId")
    
    result, err := c.queryService.GetTaskResult(ctx, taskID)
    if err != nil {
        ctx.JSON(http.StatusNotFound, gin.H{
            "code": 404,
            "message": "Result not found",
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 0,
        "data": result,
    })
}

// SSE推送任务进度
func (c *TaskController) StreamTaskProgress(ctx *gin.Context) {
    taskID := ctx.Param("taskId")
    
    // 设置SSE响应头
    ctx.Header("Content-Type", "text/event-stream")
    ctx.Header("Cache-Control", "no-cache")
    ctx.Header("Connection", "keep-alive")
    
    // 创建channel用于接收进度更新
    progressChan := make(chan service.TaskStatus)
    errorChan := make(chan error)
    
    // 启动订阅
    go func() {
        err := c.queryService.SubscribeTaskProgress(ctx, taskID, func(status service.TaskStatus) {
            progressChan <- status
        })
        errorChan <- err
    }()
    
    // 发送初始状态
    if status, err := c.queryService.GetTaskStatus(ctx, taskID); err == nil {
        ctx.SSEvent("progress", status)
        ctx.Writer.Flush()
    }
    
    // 持续推送更新
    for {
        select {
        case progress := <-progressChan:
            ctx.SSEvent("progress", progress)
            ctx.Writer.Flush()
            
            // 如果任务完成，发送完成事件并关闭
            if progress.Status == "completed" || progress.Status == "failed" {
                ctx.SSEvent("done", progress)
                ctx.Writer.Flush()
                return
            }
            
        case err := <-errorChan:
            if err != nil {
                ctx.SSEvent("error", gin.H{"error": err.Error()})
                ctx.Writer.Flush()
            }
            return
            
        case <-ctx.Done():
            return
        }
    }
}

// 恢复任务
func (c *TaskController) RecoverTask(ctx *gin.Context) {
    taskID := ctx.Param("taskId")
    
    // 检查任务是否可恢复
    canRecover, err := c.controlService.CanRecoverTask(ctx, taskID)
    if err != nil || !canRecover {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "code": 400,
            "message": "Task cannot be recovered",
        })
        return
    }
    
    // 执行恢复
    if err := c.controlService.RecoverTask(ctx, taskID); err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code": 500,
            "message": "Failed to recover task",
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 0,
        "message": "Task recovery initiated",
    })
}

// 基于已有任务创建新任务
func (c *TaskController) CreateDerivativeTask(ctx *gin.Context) {
    parentTaskID := ctx.Param("taskId")
    
    var req struct {
        Modifications map[string]interface{} `json:"modifications"`
    }
    
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "code": 400,
            "message": "Invalid request",
        })
        return
    }
    
    newTaskID, err := c.controlService.CreateDerivativeTask(ctx, parentTaskID, req.Modifications)
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code": 500,
            "message": "Failed to create derivative task",
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 0,
        "data": gin.H{
            "task_id": newTaskID,
            "parent_task_id": parentTaskID,
        },
    })
}

// 获取任务快照（用于调试）
func (c *TaskController) GetTaskSnapshot(ctx *gin.Context) {
    taskID := ctx.Param("taskId")
    version := ctx.DefaultQuery("version", "latest")
    
    snapshot, err := c.queryService.GetTaskSnapshot(ctx, taskID, version)
    if err != nil {
        ctx.JSON(http.StatusNotFound, gin.H{
            "code": 404,
            "message": "Snapshot not found",
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 0,
        "data": snapshot,
    })
}
```

### 3.3 WebSocket推送服务

```go
// backend/services/task/internal/websocket/hub.go
package websocket

import (
    "encoding/json"
    "log"
    "sync"
    
    "github.com/gorilla/websocket"
)

type Hub struct {
    clients    map[string]map[*Client]bool  // taskID -> clients
    broadcast  chan Message
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
}

type Client struct {
    hub      *Hub
    conn     *websocket.Conn
    send     chan []byte
    taskID   string
    userID   string
}

type Message struct {
    TaskID string      `json:"task_id"`
    Type   string      `json:"type"`
    Data   interface{} `json:"data"`
}

func NewHub() *Hub {
    return &Hub{
        clients:    make(map[string]map[*Client]bool),
        broadcast:  make(chan Message),
        register:   make(chan *Client),
        unregister: make(chan *Client),
    }
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            if h.clients[client.taskID] == nil {
                h.clients[client.taskID] = make(map[*Client]bool)
            }
            h.clients[client.taskID][client] = true
            h.mu.Unlock()
            
            log.Printf("Client registered: user=%s, task=%s", client.userID, client.taskID)
            
            // 发送当前状态
            go h.sendCurrentStatus(client)
            
        case client := <-h.unregister:
            h.mu.Lock()
            if clients, ok := h.clients[client.taskID]; ok {
                if _, ok := clients[client]; ok {
                    delete(clients, client)
                    close(client.send)
                    
                    if len(clients) == 0 {
                        delete(h.clients, client.taskID)
                    }
                }
            }
            h.mu.Unlock()
            
            log.Printf("Client unregistered: user=%s, task=%s", client.userID, client.taskID)
            
        case message := <-h.broadcast:
            h.mu.RLock()
            clients := h.clients[message.TaskID]
            h.mu.RUnlock()
            
            if clients != nil {
                data, _ := json.Marshal(message)
                
                for client := range clients {
                    select {
                    case client.send <- data:
                    default:
                        // 客户端阻塞，关闭连接
                        h.unregister <- client
                    }
                }
            }
        }
    }
}

func (h *Hub) NotifyProgress(taskID string, progress interface{}) {
    h.broadcast <- Message{
        TaskID: taskID,
        Type:   "progress",
        Data:   progress,
    }
}

func (h *Hub) NotifyComplete(taskID string, result interface{}) {
    h.broadcast <- Message{
        TaskID: taskID,
        Type:   "complete",
        Data:   result,
    }
}

func (h *Hub) NotifyError(taskID string, error interface{}) {
    h.broadcast <- Message{
        TaskID: taskID,
        Type:   "error",
        Data:   error,
    }
}

// websocket_handler.go
func HandleWebSocket(hub *Hub, queryService *service.TaskQueryService) gin.HandlerFunc {
    return func(c *gin.Context) {
        taskID := c.Query("task_id")
        if taskID == "" {
            c.JSON(400, gin.H{"error": "task_id required"})
            return
        }
        
        // 升级连接
        conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
        if err != nil {
            log.Println("Upgrade error:", err)
            return
        }
        
        // 创建客户端
        client := &Client{
            hub:    hub,
            conn:   conn,
            send:   make(chan []byte, 256),
            taskID: taskID,
            userID: c.GetString("user_id"),
        }
        
        client.hub.register <- client
        
        // 启动读写协程
        go client.writePump()
        go client.readPump()
    }
}
```

## 4. 数据库表设计补充

### 4.1 任务状态快照表

```sql
-- 任务状态快照表（用于回溯和调试）
CREATE TABLE `task_state_snapshots` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id` VARCHAR(100) NOT NULL COMMENT '任务ID',
  `version` INT NOT NULL COMMENT '版本号',
  `state_data` JSON NOT NULL COMMENT '完整状态数据',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_task_version` (`task_id`, `version`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务状态快照';

-- 任务进度日志表
CREATE TABLE `task_progress_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id` VARCHAR(100) NOT NULL COMMENT '任务ID',
  `phase` VARCHAR(50) NOT NULL COMMENT '阶段',
  `progress` FLOAT NOT NULL COMMENT '进度百分比',
  `message` VARCHAR(500) DEFAULT NULL COMMENT '进度消息',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_task_id` (`task_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务进度日志';

-- 任务检查点表（用于断点恢复）
CREATE TABLE `task_checkpoints` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id` VARCHAR(100) NOT NULL COMMENT '任务ID',
  `phase` VARCHAR(50) NOT NULL COMMENT '阶段',
  `checkpoint_data` LONGBLOB NOT NULL COMMENT '检查点数据',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_task_id` (`task_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务检查点';
```

## 5. 使用示例

### 5.1 前端调用示例

```typescript
// frontend/src/api/task.ts
import { axios } from '@/utils/request'

export interface TaskStatus {
  task_id: string
  status: string
  phase: string
  progress: number
  message: string
  details: Record<string, any>
  updated_at: string
}

export interface GenerationResult {
  task_id: string
  design_id: string
  images: Array<{
    url: string
    width: number
    height: number
  }>
  editable_elements: any[]
  quality_scores: Record<string, number>
}

// 查询任务状态
export function getTaskStatus(taskId: string): Promise<TaskStatus> {
  return axios.get(`/api/v1/tasks/${taskId}/status`)
}

// 获取任务结果
export function getTaskResult(taskId: string): Promise<GenerationResult> {
  return axios.get(`/api/v1/tasks/${taskId}/result`)
}

// 订阅任务进度（SSE）
export function subscribeTaskProgress(
  taskId: string,
  onProgress: (status: TaskStatus) => void,
  onComplete: (result: any) => void,
  onError: (error: any) => void
) {
  const eventSource = new EventSource(`/api/v1/tasks/${taskId}/stream`)
  
  eventSource.addEventListener('progress', (event) => {
    const status = JSON.parse(event.data)
    onProgress(status)
  })
  
  eventSource.addEventListener('complete', (event) => {
    const result = JSON.parse(event.data)
    onComplete(result)
    eventSource.close()
  })
  
  eventSource.addEventListener('error', (event) => {
    onError(event)
    eventSource.close()
  })
  
  return eventSource
}

// 使用WebSocket订阅
export function subscribeViaWebSocket(taskId: string) {
  const ws = new WebSocket(`ws://api.example.com/ws?task_id=${taskId}`)
  
  ws.onmessage = (event) => {
    const message = JSON.parse(event.data)
    
    switch (message.type) {
      case 'progress':
        console.log('Progress update:', message.data)
        break
      case 'complete':
        console.log('Task completed:', message.data)
        break
      case 'error':
        console.error('Task error:', message.data)
        break
    }
  }
  
  return ws
}

// 恢复任务
export function recoverTask(taskId: string) {
  return axios.post(`/api/v1/tasks/${taskId}/recover`)
}

// 基于已有任务创建新任务
export function createDerivativeTask(
  parentTaskId: string,
  modifications: Record<string, any>
) {
  return axios.post(`/api/v1/tasks/${parentTaskId}/derivative`, {
    modifications
  })
}
```

### 5.2 Vue组件中使用

```vue
<!-- TaskProgress.vue -->
<template>
  <div class="task-progress">
    <div class="progress-header">
      <h3>生成进度</h3>
      <el-tag :type="statusType">{{ statusText }}</el-tag>
    </div>
    
    <el-progress
      :percentage="progress"
      :status="progressStatus"
      :stroke-width="20"
    />
    
    <div class="progress-message">
      {{ currentMessage }}
    </div>
    
    <div class="progress-phases">
      <div
        v-for="phase in phases"
        :key="phase.key"
        :class="['phase-item', { active: phase.key === currentPhase }]"
      >
        <el-icon :size="20">
          <component :is="phase.icon" />
        </el-icon>
        <span>{{ phase.label }}</span>
      </div>
    </div>
    
    <div v-if="isCompleted" class="result-preview">
      <img :src="result.images[0].url" alt="生成结果" />
      <el-button type="primary" @click="openEditor">
        打开编辑器
      </el-button>
    </div>
    
    <div v-if="isFailed" class="error-actions">
      <el-alert type="error" :title="errorMessage" />
      <el-button @click="retryTask">重试</el-button>
      <el-button @click="modifyAndRetry">修改参数重试</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { 
  getTaskStatus, 
  subscribeTaskProgress, 
  getTaskResult,
  recoverTask 
} from '@/api/task'

const route = useRoute()
const taskId = route.params.taskId as string

const status = ref<TaskStatus | null>(null)
const result = ref<GenerationResult | null>(null)
const eventSource = ref<EventSource | null>(null)

const progress = computed(() => status.value?.progress || 0)
const currentPhase = computed(() => status.value?.phase || '')
const currentMessage = computed(() => status.value?.message || '准备中...')
const isCompleted = computed(() => status.value?.status === 'completed')
const isFailed = computed(() => status.value?.status === 'failed')

// 订阅进度更新
onMounted(async () => {
  // 获取初始状态
  try {
    status.value = await getTaskStatus(taskId)
  } catch (error) {
    console.error('Failed to get initial status:', error)
  }
  
  // 订阅实时更新
  eventSource.value = subscribeTaskProgress(
    taskId,
    (newStatus) => {
      status.value = newStatus
    },
    async (completion) => {
      status.value = completion
      // 获取完整结果
      result.value = await getTaskResult(taskId)
    },
    (error) => {
      console.error('Task error:', error)
      status.value = {
        ...status.value!,
        status: 'failed',
        message: error.message || '生成失败'
      }
    }
  )
})

onUnmounted(() => {
  eventSource.value?.close()
})

// 重试任务
const retryTask = async () => {
  try {
    await recoverTask(taskId)
    // 重新订阅
    location.reload()
  } catch (error) {
    ElMessage.error('重试失败')
  }
}
</script>
```

## 6. 性能优化建议

### 6.1 状态存储优化

1. **分级存储**
   - 热数据（实时进度）：Redis，TTL 7天
   - 温数据（任务结果）：Redis + MySQL
   - 冷数据（历史记录）：MySQL，定期归档

2. **数据压缩**
   - 大型中间状态使用压缩存储
   - 图片结果只存储URL和元数据

3. **批量更新**
   - 进度更新限流，避免过于频繁
   - 批量写入数据库，减少IO

### 6.2 查询优化

1. **缓存策略**
   - 结果缓存，避免重复查询
   - 使用Redis Pipeline批量查询

2. **索引优化**
   - 为高频查询字段建立索引
   - 使用复合索引优化多条件查询

3. **分页查询**
   - 历史记录使用游标分页
   - 限制单次查询数量

这个完整的状态管理方案提供了任务全生命周期的状态追踪、断点恢复、结果持久化等功能，确保了系统的可靠性和可调试性。