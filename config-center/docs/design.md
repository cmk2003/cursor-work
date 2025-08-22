# 分布式配置中心设计文档

## 一、引言

### 文档变更日志

| 版本 | 日期 | 作者 | 变更内容 |
|------|------|------|----------|
| v1.0 | 2024-01-20 | System | 初始版本，完成分布式配置中心整体设计 |
| v2.0 | 2024-01-20 | System | 添加性能优化设计：引入Redis缓存层、并发控制、请求限流、多版本配置支持 |

## 二、项目概述

### 2.1 需求背景

随着微服务架构的普及，系统中的服务数量急剧增加，每个服务都有大量的配置需要管理。传统的配置文件管理方式存在以下问题：

1. **配置分散**：配置文件分散在各个服务中，难以统一管理
2. **更新困难**：配置更新需要重启服务，影响服务可用性
3. **版本管理复杂**：难以追踪配置的历史变更
4. **缺乏权限控制**：无法对配置进行精细的权限管理
5. **高可用性要求**：单点配置服务容易成为系统瓶颈

为解决上述问题，我们设计了一个基于Raft协议的分布式配置中心，提供：
- 集中化的配置管理
- 基于namespace的配置隔离
- 实时配置推送能力
- 配置版本管理
- 高可用的分布式架构
- 高性能缓存层支持
- 并发控制与限流保护
- 多版本配置并存能力

## 三、详细设计

### 3.1 系统架构设计

#### 3.1.1 系统架构图

```mermaid
graph TB
    subgraph "客户端层"
        C1[应用服务1]
        C2[应用服务2]
        C3[应用服务N]
    end
    
    subgraph "负载均衡层"
        LB[负载均衡器<br/>Nginx/HAProxy]
    end
    
    subgraph "缓存层"
        RC[Redis集群<br/>缓存层]
    end
    
    subgraph "配置中心集群"
        subgraph "节点1"
            API1[HTTP API<br/>+限流器]
            RS1[Raft Server]
            DS1[Data Store]
        end
        
        subgraph "节点2"
            API2[HTTP API<br/>+限流器]
            RS2[Raft Server]
            DS2[Data Store]
        end
        
        subgraph "节点3"
            API3[HTTP API<br/>+限流器]
            RS3[Raft Server]
            DS3[Data Store]
        end
    end
    
    subgraph "管理端"
        Admin[管理后台]
    end
    
    C1 --> LB
    C2 --> LB
    C3 --> LB
    Admin --> LB
    
    LB --> RC
    RC --> API1
    RC --> API2
    RC --> API3
    
    API1 <--> RS1
    API2 <--> RS2
    API3 <--> RS3
    
    RS1 <--> DS1
    RS2 <--> DS2
    RS3 <--> DS3
    
    RS1 <-.-> RS2
    RS2 <-.-> RS3
    RS1 <-.-> RS3
    
    style RC fill:#faa,stroke:#333,stroke-width:2px
    style RS1 fill:#f9f,stroke:#333,stroke-width:2px
    style RS2 fill:#bbf,stroke:#333,stroke-width:2px
    style RS3 fill:#bbf,stroke:#333,stroke-width:2px
```

#### 3.1.2 Raft模块架构图

```mermaid
graph TB
    subgraph "Raft共识层"
        Leader[Leader节点]
        Follower1[Follower节点1]
        Follower2[Follower节点2]
        
        Leader -->|日志复制| Follower1
        Leader -->|日志复制| Follower2
        Follower1 -.->|心跳响应| Leader
        Follower2 -.->|心跳响应| Leader
    end
    
    subgraph "状态机层"
        SM1[状态机1]
        SM2[状态机2]
        SM3[状态机3]
    end
    
    subgraph "日志层"
        Log1[日志存储1]
        Log2[日志存储2]
        Log3[日志存储3]
    end
    
    Leader --> SM1
    Follower1 --> SM2
    Follower2 --> SM3
    
    SM1 --> Log1
    SM2 --> Log2
    SM3 --> Log3
```

#### 3.1.3 数据流架构图

```mermaid
sequenceDiagram
    participant Client
    participant LoadBalancer
    participant RateLimiter
    participant RedisCache
    participant APIServer
    participant RaftLeader
    participant RaftFollowers
    participant StateManager
    participant DataStore
    
    Client->>LoadBalancer: 1. 配置更新请求
    LoadBalancer->>RateLimiter: 2. 转发请求
    RateLimiter->>RateLimiter: 3. 限流检查
    RateLimiter->>APIServer: 4. 通过限流
    APIServer->>APIServer: 5. 参数校验
    APIServer->>RaftLeader: 6. 提交Raft日志
    RaftLeader->>RaftFollowers: 7. 日志复制
    RaftFollowers-->>RaftLeader: 8. 复制确认
    RaftLeader->>StateManager: 9. 应用到状态机
    StateManager->>DataStore: 10. 持久化数据
    StateManager->>RedisCache: 11. 更新缓存
    StateManager-->>RaftLeader: 12. 应用完成
    RaftLeader-->>APIServer: 13. 提交成功
    APIServer-->>Client: 14. 返回响应
```

### 3.2 数据库设计

#### 3.2.1 数据模型

##### 1. namespace表
```sql
CREATE TABLE namespace (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(128) NOT NULL UNIQUE COMMENT '命名空间名称',
    description VARCHAR(512) COMMENT '描述',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态: 1-启用, 0-禁用',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_name (name),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='命名空间表';
```

##### 2. config表
```sql
CREATE TABLE config (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    namespace_id BIGINT NOT NULL COMMENT '命名空间ID',
    key VARCHAR(256) NOT NULL COMMENT '配置键',
    value TEXT NOT NULL COMMENT '配置值(JSON格式)',
    version INT NOT NULL DEFAULT 1 COMMENT '版本号',
    description VARCHAR(512) COMMENT '配置描述',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态: 1-启用, 0-禁用',
    created_by VARCHAR(64) COMMENT '创建人',
    updated_by VARCHAR(64) COMMENT '更新人',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_namespace_key (namespace_id, key),
    INDEX idx_namespace_id (namespace_id),
    INDEX idx_key (key),
    INDEX idx_status (status),
    FOREIGN KEY (namespace_id) REFERENCES namespace(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='配置表';
```

##### 3. config_history表
```sql
CREATE TABLE config_history (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    config_id BIGINT NOT NULL COMMENT '配置ID',
    namespace_id BIGINT NOT NULL COMMENT '命名空间ID',
    key VARCHAR(256) NOT NULL COMMENT '配置键',
    old_value TEXT COMMENT '旧值',
    new_value TEXT NOT NULL COMMENT '新值',
    version INT NOT NULL COMMENT '版本号',
    operation VARCHAR(32) NOT NULL COMMENT '操作类型: CREATE/UPDATE/DELETE',
    operator VARCHAR(64) COMMENT '操作人',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_config_id (config_id),
    INDEX idx_namespace_id (namespace_id),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='配置历史表';
```

##### 4. raft_log表
```sql
CREATE TABLE raft_log (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    term BIGINT NOT NULL COMMENT 'Raft任期',
    index BIGINT NOT NULL UNIQUE COMMENT 'Raft日志索引',
    type VARCHAR(32) NOT NULL COMMENT '日志类型',
    data BLOB NOT NULL COMMENT '日志数据',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_term (term),
    INDEX idx_index (index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Raft日志表';
```

##### 5. raft_state表
```sql
CREATE TABLE raft_state (
    node_id VARCHAR(64) PRIMARY KEY COMMENT '节点ID',
    current_term BIGINT NOT NULL DEFAULT 0 COMMENT '当前任期',
    voted_for VARCHAR(64) COMMENT '投票给谁',
    role VARCHAR(32) NOT NULL DEFAULT 'FOLLOWER' COMMENT '角色: LEADER/CANDIDATE/FOLLOWER',
    commit_index BIGINT NOT NULL DEFAULT 0 COMMENT '已提交的日志索引',
    last_applied BIGINT NOT NULL DEFAULT 0 COMMENT '最后应用的日志索引',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Raft状态表';
```

##### 6. config_version表（新增）
```sql
CREATE TABLE config_version (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    config_id BIGINT NOT NULL COMMENT '配置ID',
    namespace_id BIGINT NOT NULL COMMENT '命名空间ID',
    key VARCHAR(256) NOT NULL COMMENT '配置键',
    value TEXT NOT NULL COMMENT '配置值(JSON格式)',
    version INT NOT NULL COMMENT '版本号',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态: 1-启用, 0-禁用',
    created_by VARCHAR(64) COMMENT '创建人',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_config_id (config_id),
    INDEX idx_namespace_key_version (namespace_id, key, version),
    INDEX idx_status (status),
    UNIQUE KEY uk_config_version (config_id, version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='配置版本表，存储所有历史版本';
```

### 3.3 系统功能设计

#### 3.3.1 功能模块划分

```mermaid
graph LR
    subgraph "配置管理模块"
        CM1[配置CRUD]
        CM2[版本管理]
        CM3[配置回滚]
        CM4[批量操作]
        CM5[多版本并存]
    end
    
    subgraph "命名空间模块"
        NS1[命名空间管理]
        NS2[权限隔离]
    end
    
    subgraph "客户端接口模块"
        CI1[配置拉取]
        CI2[配置监听]
        CI3[缓存管理]
        CI4[版本选择]
    end
    
    subgraph "分布式模块"
        DM1[Raft选举]
        DM2[日志复制]
        DM3[数据同步]
        DM4[故障转移]
    end
    
    subgraph "性能优化模块"
        PM1[Redis缓存]
        PM2[并发控制]
        PM3[请求限流]
        PM4[缓存更新]
    end
    
    subgraph "监控模块"
        MM1[性能监控]
        MM2[操作审计]
        MM3[告警通知]
    end
```

#### 3.3.2 核心功能流程

##### 1. 配置创建流程

```mermaid
flowchart TB
    Start([开始]) --> ValidateInput{参数校验}
    ValidateInput -->|不通过| ReturnError[返回错误]
    ValidateInput -->|通过| CheckNamespace{命名空间是否存在}
    CheckNamespace -->|不存在| ReturnError
    CheckNamespace -->|存在| CheckKey{配置键是否存在}
    CheckKey -->|存在| ReturnError
    CheckKey -->|不存在| SubmitRaft[提交到Raft]
    SubmitRaft --> WaitConsensus{等待共识}
    WaitConsensus -->|超时| ReturnError
    WaitConsensus -->|成功| ApplyStateMachine[应用到状态机]
    ApplyStateMachine --> SaveDB[保存到数据库]
    SaveDB --> SaveHistory[记录历史]
    SaveHistory --> NotifyClients[通知客户端]
    NotifyClients --> ReturnSuccess[返回成功]
    ReturnError --> End([结束])
    ReturnSuccess --> End
```

##### 2. 配置读取流程（优化后）

```mermaid
flowchart TB
    Start([开始]) --> CheckRedis{检查Redis缓存}
    CheckRedis -->|命中| ReturnRedis[返回缓存数据]
    CheckRedis -->|未命中| CheckLocalCache{检查本地缓存}
    CheckLocalCache -->|命中| CheckVersion{检查版本}
    CheckLocalCache -->|未命中| CheckRole{检查节点角色}
    CheckVersion -->|版本匹配| ReturnLocal[返回本地缓存]
    CheckVersion -->|版本不匹配| CheckRole
    CheckRole -->|Leader| QueryDB[查询数据库]
    CheckRole -->|Follower| ForwardLeader{转发到Leader}
    ForwardLeader -->|成功| QueryDB
    ForwardLeader -->|失败| QueryLocalDB[查询本地数据库]
    QueryDB --> UpdateRedis[更新Redis缓存]
    QueryLocalDB --> CheckStale{检查数据时效}
    CheckStale -->|过期| ReturnError[返回错误]
    CheckStale -->|有效| UpdateRedis
    UpdateRedis --> UpdateLocal[更新本地缓存]
    UpdateLocal --> ReturnData[返回数据]
    ReturnRedis --> End([结束])
    ReturnLocal --> End
    ReturnData --> End
    ReturnError --> End
```

##### 3. Raft选举流程

```mermaid
stateDiagram-v2
    [*] --> Follower: 初始状态
    
    Follower --> Candidate: 选举超时
    
    Candidate --> Candidate: 分票,重新选举
    Candidate --> Leader: 获得多数票
    Candidate --> Follower: 发现更高任期
    
    Leader --> Follower: 发现更高任期
    
    state Follower {
        [*] --> 等待心跳
        等待心跳 --> 响应投票请求
        响应投票请求 --> 等待心跳
        等待心跳 --> [*]: 超时
    }
    
    state Candidate {
        [*] --> 增加任期
        增加任期 --> 投票给自己
        投票给自己 --> 发送投票请求
        发送投票请求 --> 等待响应
        等待响应 --> [*]: 获得多数票或超时
    }
    
    state Leader {
        [*] --> 发送心跳
        发送心跳 --> 处理客户端请求
        处理客户端请求 --> 日志复制
        日志复制 --> 发送心跳
    }
```

### 3.4 缓存设计（新增）

#### 3.4.1 Redis缓存策略

##### 1. 缓存数据结构设计

```
# 配置缓存
Key: config:{namespace}:{key}
Value: {
    "value": {...},      # 配置值
    "version": 3,        # 当前版本
    "updated_at": "...", # 更新时间
    "ttl": 3600         # 过期时间(秒)
}
TTL: 1小时（热点数据自动续期）

# 配置版本缓存
Key: config:version:{namespace}:{key}:{version}
Value: {
    "value": {...},      # 该版本的配置值
    "created_at": "..."  # 创建时间
}
TTL: 24小时

# 命名空间配置集合
Key: namespace:configs:{namespace}
Type: Hash
Field: {key}
Value: {
    "version": 3,
    "value": {...}
}
TTL: 1小时

# 配置版本列表
Key: config:versions:{namespace}:{key}
Type: Sorted Set
Member: version
Score: timestamp
TTL: 永不过期
```

##### 2. 缓存更新策略

```mermaid
flowchart LR
    subgraph "写入路径"
        Write[配置更新] --> Raft[Raft共识]
        Raft --> DB[数据库写入]
        DB --> InvalidateCache[缓存失效]
        InvalidateCache --> PublishEvent[发布更新事件]
    end
    
    subgraph "读取路径"
        Read[配置读取] --> CheckCache{缓存检查}
        CheckCache -->|命中| ReturnCache[返回缓存]
        CheckCache -->|未命中| LoadDB[加载数据库]
        LoadDB --> UpdateCache[更新缓存]
        UpdateCache --> ReturnData[返回数据]
    end
    
    subgraph "事件处理"
        PublishEvent --> Subscriber[订阅者]
        Subscriber --> RefreshLocal[刷新本地缓存]
    end
```

#### 3.4.2 缓存一致性保证

##### 1. 缓存更新机制

采用**Cache-Aside Pattern**结合**发布订阅**模式：
- 写操作：先更新数据库，再删除缓存，最后发布更新事件
- 读操作：先查缓存，未命中则查数据库并更新缓存
- 事件通知：通过Redis Pub/Sub通知所有节点更新本地缓存

##### 2. 分布式锁机制

```go
// 使用Redis分布式锁防止缓存击穿
lockKey := fmt.Sprintf("lock:config:%s:%s", namespace, key)
lock := redis.SetNX(lockKey, nodeID, 5*time.Second)
if lock {
    defer redis.Del(lockKey)
    // 加载数据并更新缓存
}
```

##### 3. 缓存预热

```mermaid
flowchart TB
    Start[服务启动] --> LoadHotConfigs[加载热点配置列表]
    LoadHotConfigs --> BatchLoad[批量加载配置]
    BatchLoad --> WarmCache[预热Redis缓存]
    WarmCache --> WarmLocal[预热本地缓存]
    WarmLocal --> Ready[服务就绪]
```

### 3.5 并发控制与限流设计（新增）

#### 3.5.1 并发控制

##### 1. 请求级并发控制

```go
// 使用令牌桶算法实现请求限流
type RateLimiter struct {
    rate       int           // 每秒允许的请求数
    capacity   int           // 桶容量
    tokens     int           // 当前令牌数
    lastUpdate time.Time     // 上次更新时间
    mu         sync.Mutex    // 互斥锁
}

// 全局限流配置
var limiters = map[string]*RateLimiter{
    "read":  NewRateLimiter(10000, 20000),  // 读请求：10000 QPS
    "write": NewRateLimiter(1000, 2000),    // 写请求：1000 QPS
    "admin": NewRateLimiter(100, 200),      // 管理请求：100 QPS
}
```

##### 2. 客户端级限流

```yaml
# 限流规则配置
rate_limits:
  default:
    read_qps: 1000      # 默认每客户端读QPS
    write_qps: 100      # 默认每客户端写QPS
  
  vip_clients:         # VIP客户端配置
    - client_id: "core-service"
      read_qps: 5000
      write_qps: 500
```

#### 3.5.2 限流实现

##### 1. 限流中间件

```mermaid
flowchart TB
    Request[请求到达] --> GetClientID[获取客户端ID]
    GetClientID --> CheckGlobal{全局限流检查}
    CheckGlobal -->|超限| RejectGlobal[返回429错误]
    CheckGlobal -->|通过| CheckClient{客户端限流检查}
    CheckClient -->|超限| RejectClient[返回429错误]
    CheckClient -->|通过| ProcessRequest[处理请求]
    ProcessRequest --> Response[返回响应]
    RejectGlobal --> End[结束]
    RejectClient --> End
    Response --> End
```

##### 2. 限流响应

```json
{
    "code": 42901,
    "message": "请求过于频繁，请稍后重试",
    "error": {
        "retry_after": 1,        // 建议重试时间(秒)
        "limit": 1000,          // 限流阈值
        "remaining": 0,         // 剩余配额
        "reset": 1705734460     // 配额重置时间
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

### 3.6 多版本配置管理（新增）

#### 3.6.1 版本管理策略

##### 1. 版本存储

- 每次配置更新生成新版本，保留历史版本
- 支持按版本号、时间戳查询历史配置
- 可配置版本保留策略（如保留最近30个版本）

##### 2. 版本清理策略

```mermaid
flowchart LR
    subgraph "版本保留策略"
        VS1[保留最新N个版本]
        VS2[保留最近M天版本]
        VS3[保留标记版本]
    end
    
    subgraph "清理任务"
        CT1[定时清理任务]
        CT2[手动清理触发]
    end
    
    VS1 --> CleanupLogic[清理逻辑]
    VS2 --> CleanupLogic
    VS3 --> CleanupLogic
    CT1 --> CleanupLogic
    CT2 --> CleanupLogic
    CleanupLogic --> Archive[归档历史数据]
    CleanupLogic --> Delete[删除过期版本]
```

#### 3.6.2 版本访问接口

##### 1. 指定版本获取

```bash
# 获取特定版本配置
GET /api/v1/config/get?namespace=user-service&key=database.mysql&version=3
```

##### 2. 版本范围查询

```bash
# 获取版本列表
GET /api/v1/config/versions?namespace=user-service&key=database.mysql&limit=10
```

##### 3. 版本对比

```bash
# 对比两个版本差异
GET /api/v1/config/diff?namespace=user-service&key=database.mysql&from=3&to=5
```

### 3.7 接口设计

#### 3.7.1 API设计原则

遵循365 API规范：
1. RESTful风格设计
2. 统一的请求/响应格式
3. 标准的HTTP状态码
4. 版本化管理
5. 统一的错误处理

#### 3.7.2 统一响应格式

```json
{
    "code": 0,
    "message": "success",
    "data": {},
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

错误响应格式：
```json
{
    "code": 40001,
    "message": "参数错误",
    "error": {
        "details": "namespace不能为空"
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

#### 3.7.3 客户端配置读取接口

##### 1. 获取单个配置

**接口地址**: `GET /api/v1/config/get`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 是 | 命名空间 |
| key | string | 是 | 配置键 |
| version | int | 否 | 配置版本号，不传则返回最新版本 |

**请求示例**:
```bash
# 获取最新版本
GET /api/v1/config/get?namespace=user-service&key=database.mysql.host

# 获取指定版本
GET /api/v1/config/get?namespace=user-service&key=database.mysql.host&version=3
```

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "namespace": "user-service",
        "key": "database.mysql.host",
        "value": {
            "host": "192.168.1.100",
            "port": 3306,
            "username": "root",
            "database": "user_db"
        },
        "version": 3,
        "updated_at": "2024-01-20T10:00:00Z"
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 2. 批量获取配置

**接口地址**: `POST /api/v1/config/batch-get`

**请求参数**:
```json
{
    "namespace": "user-service",
    "keys": ["database.mysql", "redis.cluster", "app.settings"]
}
```

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "namespace": "user-service",
        "configs": [
            {
                "key": "database.mysql",
                "value": {
                    "host": "192.168.1.100",
                    "port": 3306
                },
                "version": 3
            },
            {
                "key": "redis.cluster",
                "value": {
                    "nodes": ["192.168.1.101:6379", "192.168.1.102:6379"]
                },
                "version": 2
            }
        ]
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 3. 获取命名空间所有配置

**接口地址**: `GET /api/v1/config/namespace/{namespace}`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 是 | 命名空间(路径参数) |

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "namespace": "user-service",
        "configs": {
            "database.mysql": {
                "host": "192.168.1.100",
                "port": 3306
            },
            "redis.cluster": {
                "nodes": ["192.168.1.101:6379"]
            },
            "app.settings": {
                "timeout": 30,
                "retry": 3
            }
        },
        "version": 15,
        "updated_at": "2024-01-20T10:00:00Z"
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

#### 3.7.4 后台管理接口（增强版本管理）

##### 1. 创建配置

**接口地址**: `POST /api/v1/admin/config`

**请求参数**:
```json
{
    "namespace": "user-service",
    "key": "new.config.key",
    "value": {
        "setting1": "value1",
        "setting2": 123
    },
    "description": "新配置项描述"
}
```

**响应示例**:
```json
{
    "code": 0,
    "message": "配置创建成功",
    "data": {
        "id": 12345,
        "namespace": "user-service",
        "key": "new.config.key",
        "version": 1
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 2. 更新配置

**接口地址**: `PUT /api/v1/admin/config`

**请求参数**:
```json
{
    "namespace": "user-service",
    "key": "database.mysql",
    "value": {
        "host": "192.168.1.200",
        "port": 3306
    },
    "description": "更新数据库地址"
}
```

**响应示例**:
```json
{
    "code": 0,
    "message": "配置更新成功",
    "data": {
        "id": 12345,
        "namespace": "user-service",
        "key": "database.mysql",
        "version": 4,
        "old_version": 3
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 3. 删除配置

**接口地址**: `DELETE /api/v1/admin/config`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 是 | 命名空间 |
| key | string | 是 | 配置键 |

**响应示例**:
```json
{
    "code": 0,
    "message": "配置删除成功",
    "data": {
        "namespace": "user-service",
        "key": "deprecated.config"
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 4. 查询配置列表

**接口地址**: `GET /api/v1/admin/config/list`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 否 | 命名空间过滤 |
| key | string | 否 | 配置键模糊搜索 |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认20 |

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "list": [
            {
                "id": 12345,
                "namespace": "user-service",
                "key": "database.mysql",
                "value": {
                    "host": "192.168.1.100"
                },
                "version": 3,
                "description": "MySQL配置",
                "status": 1,
                "updated_at": "2024-01-20T10:00:00Z",
                "updated_by": "admin"
            }
        ],
        "pagination": {
            "page": 1,
            "page_size": 20,
            "total": 100,
            "total_pages": 5
        }
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 5. 查询配置历史

**接口地址**: `GET /api/v1/admin/config/history`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 是 | 命名空间 |
| key | string | 是 | 配置键 |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认20 |

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "list": [
            {
                "id": 1001,
                "namespace": "user-service",
                "key": "database.mysql",
                "old_value": {
                    "host": "192.168.1.100"
                },
                "new_value": {
                    "host": "192.168.1.200"
                },
                "version": 4,
                "operation": "UPDATE",
                "operator": "admin",
                "created_at": "2024-01-20T10:00:00Z"
            }
        ],
        "pagination": {
            "page": 1,
            "page_size": 20,
            "total": 15
        }
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 6. 配置回滚

**接口地址**: `POST /api/v1/admin/config/rollback`

**请求参数**:
```json
{
    "namespace": "user-service",
    "key": "database.mysql",
    "target_version": 3
}
```

**响应示例**:
```json
{
    "code": 0,
    "message": "配置回滚成功",
    "data": {
        "namespace": "user-service",
        "key": "database.mysql",
        "current_version": 5,
        "previous_version": 4
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

#### 3.7.5 版本管理接口（新增）

##### 1. 获取配置版本列表

**接口地址**: `GET /api/v1/config/versions`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 是 | 命名空间 |
| key | string | 是 | 配置键 |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认20 |

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "list": [
            {
                "version": 5,
                "value": {
                    "host": "192.168.1.200"
                },
                "created_by": "admin",
                "created_at": "2024-01-20T12:00:00Z",
                "is_current": true
            },
            {
                "version": 4,
                "value": {
                    "host": "192.168.1.150"
                },
                "created_by": "admin",
                "created_at": "2024-01-20T11:00:00Z",
                "is_current": false
            }
        ],
        "pagination": {
            "page": 1,
            "page_size": 20,
            "total": 5
        }
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 2. 配置版本对比

**接口地址**: `GET /api/v1/config/diff`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| namespace | string | 是 | 命名空间 |
| key | string | 是 | 配置键 |
| from_version | int | 是 | 起始版本号 |
| to_version | int | 是 | 目标版本号 |

**响应示例**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "namespace": "user-service",
        "key": "database.mysql",
        "from_version": 3,
        "to_version": 5,
        "changes": [
            {
                "path": "host",
                "from": "192.168.1.100",
                "to": "192.168.1.200",
                "type": "modified"
            },
            {
                "path": "max_connections",
                "from": null,
                "to": 100,
                "type": "added"
            }
        ]
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

#### 3.7.6 命名空间管理接口

##### 1. 创建命名空间

**接口地址**: `POST /api/v1/admin/namespace`

**请求参数**:
```json
{
    "name": "order-service",
    "description": "订单服务配置命名空间"
}
```

**响应示例**:
```json
{
    "code": 0,
    "message": "命名空间创建成功",
    "data": {
        "id": 10,
        "name": "order-service",
        "description": "订单服务配置命名空间",
        "status": 1,
        "created_at": "2024-01-20T10:00:00Z"
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": 1705734400
}
```

##### 2. 更新命名空间

**接口地址**: `PUT /api/v1/admin/namespace/{id}`

**请求参数**:
```json
{
    "description": "更新后的描述",
    "status": 1
}
```

##### 3. 删除命名空间

**接口地址**: `DELETE /api/v1/admin/namespace/{id}`

**注意**: 只能删除没有配置项的空命名空间

##### 4. 查询命名空间列表

**接口地址**: `GET /api/v1/admin/namespace/list`

**请求参数**:
| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| name | string | 否 | 名称模糊搜索 |
| status | int | 否 | 状态过滤 |
| page | int | 否 | 页码 |
| page_size | int | 否 | 每页数量 |

#### 3.7.7 错误码定义（更新）

| 错误码 | 说明 | HTTP状态码 |
|--------|------|-------------|
| 0 | 成功 | 200 |
| 40001 | 参数错误 | 400 |
| 40002 | 参数校验失败 | 400 |
| 40101 | 未授权 | 401 |
| 40301 | 无权限 | 403 |
| 40401 | 资源不存在 | 404 |
| 40402 | 命名空间不存在 | 404 |
| 40403 | 配置项不存在 | 404 |
| 40901 | 资源冲突 | 409 |
| 40902 | 配置键已存在 | 409 |
| 50001 | 服务内部错误 | 500 |
| 50002 | 数据库错误 | 500 |
| 50003 | Raft共识失败 | 500 |
| 50004 | 缓存服务异常 | 500 |
| 42901 | 请求频率超限 | 429 |
| 42902 | 客户端配额超限 | 429 |
| 50301 | 服务不可用 | 503 |
| 50302 | 节点非Leader | 503 |

## 五、附录

### 5.1 名词解释

| 术语 | 说明 |
|------|------|
| Namespace | 命名空间，用于隔离不同业务或服务的配置 |
| Config Key | 配置键，配置项的唯一标识 |
| Config Value | 配置值，支持JSON格式的复杂数据结构 |
| Raft | 一种分布式一致性算法，用于保证集群数据一致性 |
| Leader | Raft集群中的领导者节点，负责处理所有写请求 |
| Follower | Raft集群中的跟随者节点，负责同步Leader的数据 |
| Candidate | Raft选举过程中的候选者节点 |
| Term | Raft协议中的任期概念，每次选举会增加任期号 |
| Log Entry | Raft日志条目，记录每个操作命令 |
| State Machine | 状态机，将Raft日志应用到实际业务数据的组件 |
| Redis | 开源的内存数据结构存储系统，用作缓存层 |
| Cache-Aside Pattern | 缓存模式，应用程序负责从数据源加载数据到缓存 |
| Rate Limiting | 限流，控制请求频率的技术 |
| Token Bucket | 令牌桶算法，一种常用的限流算法 |
| TTL | Time To Live，数据的存活时间 |
| QPS | Queries Per Second，每秒查询数 |

### 5.2 参考资料

1. [Raft一致性算法论文](https://raft.github.io/raft.pdf)
2. [etcd官方文档](https://etcd.io/docs/)
3. [Apollo配置中心](https://github.com/apolloconfig/apollo)
4. [Consul文档](https://www.consul.io/docs)
5. [分布式系统一致性](https://www.allthingsdistributed.com/2008/12/eventually_consistent.html)
6. [CAP理论](https://en.wikipedia.org/wiki/CAP_theorem)
7. [365 API设计规范](https://365.design/api-guidelines)
8. [Redis官方文档](https://redis.io/documentation)
9. [分布式缓存最佳实践](https://docs.microsoft.com/en-us/azure/architecture/best-practices/caching)
10. [Rate Limiting算法详解](https://www.cloudflare.com/learning/bots/what-is-rate-limiting/)
11. [高并发系统设计](https://github.com/donnemartin/system-design-primer)