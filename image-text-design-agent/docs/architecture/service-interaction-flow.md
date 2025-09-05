# WPS AI设计室 - 服务调用关系详解

## 1. 服务间调用关系总览

```mermaid
graph TB
    subgraph "客户端层"
        Web[Web前端]
    end
    
    subgraph "网关层"
        Gateway[API Gateway]
        WSGateway[WebSocket Gateway]
    end
    
    subgraph "业务服务层"
        US[User Service]
        PS[Project Service]
        DS[Design Service]
        CS[Conversation Service]
        TS[Task Service]
        MS[Material Service]
        TempS[Template Service]
        FS[File Service]
    end
    
    subgraph "算法层"
        AS[Algorithm Service]
    end
    
    subgraph "消息队列"
        Kafka[Kafka]
    end
    
    %% 客户端到网关
    Web -->|HTTP| Gateway
    Web -->|WebSocket| WSGateway
    
    %% 网关到服务
    Gateway --> US
    Gateway --> PS
    Gateway --> DS
    Gateway --> CS
    Gateway --> MS
    Gateway --> TempS
    Gateway --> FS
    WSGateway --> CS
    
    %% 服务间同步调用
    PS -->|验证用户| US
    PS -->|创建对话| CS
    
    CS -->|获取项目信息| PS
    CS -->|创建任务| TS
    CS -->|获取设计| DS
    CS -->|获取素材信息| MS
    
    DS -->|验证项目归属| PS
    DS -->|上传文件| FS
    DS -->|获取素材| MS
    
    MS -->|上传文件| FS
    MS -->|验证用户| US
    
    TempS -->|获取设计数据| DS
    TempS -->|创建项目| PS
    
    %% 异步调用
    TS -->|发送任务| Kafka
    AS -->|消费任务| Kafka
    
    %% 回调
    AS -.->|更新进度| TS
    AS -.->|保存结果| DS
    TS -.->|通知完成| CS
    CS -.->|推送消息| WSGateway
    
    style Web fill:#e1f5fe
    style Gateway fill:#fff3e0
    style WSGateway fill:#fff3e0
    style US fill:#e8f5e9
    style PS fill:#e8f5e9
    style DS fill:#e8f5e9
    style CS fill:#e8f5e9
    style TS fill:#e8f5e9
    style MS fill:#f3e5f5
    style TempS fill:#f3e5f5
    style FS fill:#f3e5f5
    style AS fill:#fff9c4
    style Kafka fill:#ffebee
```

## 2. 核心业务流程中的服务调用

### 2.1 创建项目并开始设计

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant GW as API Gateway
    participant US as User Service
    participant PS as Project Service
    participant CS as Conversation Service
    participant MS as Material Service
    participant FS as File Service
    
    Web->>GW: POST /api/v1/projects
    Note over Web: 提交提示词、物料类型、<br/>尺寸、参考图、素材等
    
    GW->>US: 验证用户身份
    US-->>GW: 用户信息
    
    GW->>PS: 创建项目
    
    alt 有素材上传
        PS->>MS: 获取素材信息
        MS->>FS: 验证文件存在
        FS-->>MS: 文件信息
        MS-->>PS: 素材详情
    end
    
    PS->>PS: 保存项目数据
    PS->>CS: 创建对话会话
    CS->>CS: 初始化对话上下文
    CS-->>PS: 会话ID
    
    PS-->>GW: 项目信息 + 会话ID
    GW-->>Web: 返回结果
    
    Web->>WSG: 建立WebSocket连接
    Note over Web: 使用会话ID连接
```

### 2.2 意图补全对话流程

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant WSG as WebSocket Gateway
    participant CS as Conversation Service
    participant PS as Project Service
    participant MS as Material Service
    participant FS as File Service
    
    CS->>CS: 分析初始意图
    CS->>PS: 获取项目详情
    PS-->>CS: 项目信息
    
    alt 需要补全信息
        loop 最多3轮对话
            CS->>WSG: 推送补全提示
            WSG->>Web: 显示AI提问
            Web->>WSG: 用户回复
            
            alt 用户上传附件
                Web->>GW: 上传文件
                GW->>FS: 处理上传
                FS-->>GW: 文件URL
                GW-->>Web: 上传成功
            end
            
            WSG->>CS: 转发用户回复
            CS->>CS: 更新对话上下文
            
            alt 信息已完整
                CS->>CS: 标记意图完整
                Note over CS: 跳出循环
            end
        end
    end
    
    CS->>WSG: 推送意图确认完成
    WSG->>Web: 显示确认信息
```

### 2.3 生成任务执行流程

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant CS as Conversation Service
    participant TS as Task Service
    participant Kafka as Kafka
    participant AS as Algorithm Service
    participant DS as Design Service
    participant WSG as WebSocket Gateway
    
    CS->>TS: 创建生成任务
    Note over CS: 包含完整的意图信息
    
    TS->>TS: 保存任务
    TS->>Kafka: 发布任务消息
    TS-->>CS: 返回任务ID
    
    CS->>WSG: 推送"开始生成"
    WSG->>Web: 显示生成中状态
    
    AS->>Kafka: 消费任务
    AS->>TS: 获取任务详情
    TS-->>AS: 完整任务数据
    
    loop 生成过程
        AS->>AS: 执行生成步骤
        AS->>TS: 更新进度
        TS->>CS: 通知进度更新
        CS->>WSG: 推送进度
        WSG->>Web: 更新进度条
    end
    
    AS->>AS: 生成完成
    Note over AS: 生成3个备选方案
    
    AS->>DS: 保存设计结果
    DS->>DS: 创建设计记录
    DS-->>AS: 设计ID列表
    
    AS->>TS: 标记任务完成
    TS->>CS: 通知完成
    CS->>DS: 获取设计详情
    DS-->>CS: 设计数据
    
    CS->>WSG: 推送生成结果
    WSG->>Web: 显示生成的设计
```

### 2.4 多轮对话修改流程

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant WSG as WebSocket Gateway
    participant CS as Conversation Service
    participant DS as Design Service
    participant TS as Task Service
    participant AS as Algorithm Service
    
    Web->>WSG: 发送修改指令
    WSG->>CS: 转发消息
    
    CS->>CS: 解析修改意图
    
    alt 未选中具体设计
        CS->>DS: 获取当前所有设计
        DS-->>CS: 设计列表
        CS->>WSG: 询问修改哪张
        WSG->>Web: 显示选择提示
        Web->>WSG: 选择目标
        WSG->>CS: 用户选择
    end
    
    CS->>DS: 获取目标设计详情
    DS-->>CS: 设计数据
    
    CS->>TS: 创建修改任务
    Note over CS: 任务类型为modify
    
    TS->>Kafka: 发布修改任务
    
    %% 算法处理
    AS->>Kafka: 消费任务
    AS->>DS: 获取原始设计
    DS-->>AS: 设计数据
    
    AS->>AS: 执行修改
    AS->>DS: 保存新版本
    DS->>DS: 创建新设计记录
    
    AS->>TS: 完成任务
    TS->>CS: 通知完成
    CS->>WSG: 推送新设计
    WSG->>Web: 显示修改结果
```

### 2.5 图上编辑流程

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant GW as API Gateway
    participant DS as Design Service
    participant MS as Material Service
    participant FS as File Service
    participant PS as Project Service
    
    Web->>Web: 本地画布操作
    Note over Web: 移动、缩放、旋转等
    
    Web->>GW: PUT /api/v1/designs/:id
    Note over Web: 批量提交修改
    
    GW->>DS: 更新设计
    DS->>PS: 验证项目归属
    PS-->>DS: 验证通过
    
    alt 添加文字
        DS->>DS: 创建文字元素
    else 添加图片
        Web->>GW: POST /api/v1/files/upload
        GW->>FS: 上传图片
        FS->>FS: 处理图片
        FS-->>GW: 图片URL
        GW-->>Web: 上传结果
        
        Web->>GW: POST /api/v1/designs/:id/elements
        GW->>DS: 添加图片元素
        DS->>MS: 记录素材使用
    else 修改元素属性
        DS->>DS: 更新元素数据
    end
    
    DS->>DS: 保存新版本
    DS-->>GW: 更新成功
    GW-->>Web: 返回结果
```

### 2.6 使用模板流程

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant GW as API Gateway
    participant TempS as Template Service
    participant PS as Project Service
    participant CS as Conversation Service
    participant DS as Design Service
    
    Web->>GW: GET /api/v1/templates
    GW->>TempS: 获取模板列表
    TempS-->>GW: 模板数据
    GW-->>Web: 显示模板
    
    Web->>GW: POST /api/v1/templates/:id/use
    GW->>TempS: 使用模板
    
    TempS->>TempS: 获取模板数据
    TempS->>PS: 创建项目
    Note over TempS: 使用模板的提示词和参数
    
    PS->>CS: 创建对话会话
    CS-->>PS: 会话ID
    PS-->>TempS: 项目ID
    
    alt 模板包含样例设计
        TempS->>DS: 复制样例设计
        DS-->>TempS: 新设计ID
    end
    
    TempS->>TempS: 更新使用统计
    TempS-->>GW: 返回项目信息
    GW-->>Web: 跳转到编辑页
```

### 2.7 导出设计流程

```mermaid
sequenceDiagram
    participant Web as Web前端
    participant GW as API Gateway
    participant DS as Design Service
    participant FS as File Service
    
    Web->>GW: POST /api/v1/designs/:id/export
    Note over Web: 指定格式、分辨率等
    
    GW->>DS: 请求导出
    DS->>DS: 获取设计数据
    
    alt 高清导出
        DS->>FS: 请求图片超分
        FS->>FS: 处理背景图
        FS-->>DS: 高清背景
    end
    
    DS->>DS: 渲染画布
    Note over DS: 合成所有元素
    
    DS->>FS: 保存导出文件
    FS->>FS: 生成临时文件
    FS-->>DS: 文件URL
    
    DS-->>GW: 返回下载链接
    GW-->>Web: 开始下载
    
    Web->>GW: GET [下载链接]
    GW->>FS: 获取文件
    FS-->>GW: 文件流
    GW-->>Web: 下载文件
```

## 3. 服务间调用方式

### 3.1 同步调用（HTTP/gRPC）

| 调用方 | 被调用方 | 调用场景 | 接口类型 |
|--------|----------|----------|----------|
| Gateway | All Services | 所有客户端请求 | HTTP REST |
| Project Service | User Service | 验证用户权限 | gRPC |
| Project Service | Conversation Service | 创建对话会话 | gRPC |
| Conversation Service | Project Service | 获取项目信息 | gRPC |
| Conversation Service | Task Service | 创建任务 | gRPC |
| Conversation Service | Design Service | 获取/创建设计 | gRPC |
| Design Service | File Service | 文件处理 | gRPC |
| Design Service | Material Service | 素材管理 | gRPC |
| Material Service | File Service | 文件上传 | gRPC |
| Template Service | Project Service | 创建项目 | gRPC |
| Template Service | Design Service | 复制设计 | gRPC |

### 3.2 异步调用（Kafka）

| 生产者 | 消费者 | 消息类型 | 用途 |
|--------|---------|----------|------|
| Task Service | Algorithm Service | 生成任务 | 图文生成 |
| Task Service | Algorithm Service | 修改任务 | 设计修改 |
| Algorithm Service | Task Service | 进度更新 | 状态同步 |
| Algorithm Service | Design Service | 结果保存 | 保存生成结果 |

### 3.3 实时推送（WebSocket）

| 推送方 | 接收方 | 推送内容 |
|--------|---------|----------|
| Conversation Service | Web前端 | 对话消息、生成进度、任务状态 |
| Task Service | Conversation Service | 任务进度更新 |

## 4. 服务调用的关键原则

1. **最小依赖原则**：每个服务只依赖必要的其他服务
2. **异步优先**：耗时操作使用消息队列异步处理
3. **缓存策略**：高频访问的数据使用Redis缓存
4. **熔断保护**：服务间调用实现熔断机制
5. **幂等设计**：所有写操作支持幂等

## 5. 数据一致性保障

### 5.1 事务边界

- 单服务内：使用数据库事务
- 跨服务：使用Saga模式或最终一致性

### 5.2 补偿机制

```mermaid
graph LR
    A[创建项目] --> B[创建对话]
    B --> C[创建任务]
    C --> D{失败?}
    D -->|是| E[删除对话]
    E --> F[删除项目]
    D -->|否| G[继续流程]
```

这个服务调用关系设计确保了系统的高内聚低耦合，每个服务都有明确的职责和边界，通过合理的同步/异步调用组合，实现了良好的性能和用户体验。