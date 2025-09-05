# AI图文设计Agent系统 - 服务端与算法端交互设计

## 项目概述

本项目聚焦于AI图文设计Agent系统中服务端（Go）与算法端（Python）的交互设计，重点关注海报生成任务的状态管理和元素存储。

## 系统架构

详见 [服务端与算法端交互设计文档](docs/architecture/backend-algorithm-interaction.md)

### 核心组件

1. **服务端（Go）**
   - 任务服务：管理生成任务的生命周期
   - 海报服务：存储海报信息和图层关系
   - 元素服务：管理海报中的各个设计元素
   - 存储层：MySQL持久化 + Redis缓存 + OSS对象存储

2. **算法端（Python）**
   - 算法调度服务：消费任务队列
   - LangGraph工作流：执行AI生成流程
   - 后端客户端SDK：调用服务端API

3. **消息队列**
   - Kafka：任务分发和异步处理

## 核心数据模型

### 任务（Task）
- 存储用户的生成请求和参数
- 跟踪任务状态和进度
- 记录生成结果或错误信息

### 海报（Poster）
- 海报基本信息（尺寸、背景等）
- 关联的图层列表
- 预览图和源文件URL

### 元素（Element）
- 文字、图片、形状等设计元素
- 位置、样式、内容等属性
- 支持图层管理和变换操作

## API接口

### 任务管理
- `POST /api/v1/tasks` - 创建任务
- `GET /api/v1/tasks/:taskId/status` - 查询状态
- `PUT /api/v1/tasks/:taskId/progress` - 更新进度（算法端调用）
- `POST /api/v1/tasks/:taskId/complete` - 完成任务（算法端调用）

### 海报管理
- `POST /api/v1/posters` - 创建海报（算法端调用）
- `GET /api/v1/posters/:posterId` - 获取海报详情
- `GET /api/v1/posters/:posterId/full` - 获取海报及所有元素

### 元素管理
- `POST /api/v1/elements/batch` - 批量创建元素（算法端调用）
- `GET /api/v1/elements/:elementId` - 获取元素详情
- `PUT /api/v1/elements/:elementId` - 更新元素

## 快速开始

### 环境要求
- Go 1.21+
- Python 3.11+
- MySQL 8.0+
- Redis 7.0+
- Kafka 3.0+

### 数据库初始化
```bash
mysql -u root -p < docs/scripts/init.sql
```

### 启动服务
```bash
# 启动服务端
cd backend
go run cmd/main.go

# 启动算法端
cd algorithm
python main.py
```

## 文档结构

```
docs/
├── architecture/
│   ├── system-architecture.md      # 总体架构设计
│   ├── backend-design.md          # 服务端技术方案
│   ├── algorithm-design.md        # 算法端技术方案
│   └── backend-algorithm-interaction.md  # 交互设计（核心文档）
├── api/
│   └── api-design.md              # API接口文档
└── database/
    └── database-design.md         # 数据库设计
```

## 项目特点

1. **清晰的职责分离**：服务端负责存储和状态管理，算法端专注于AI生成
2. **灵活的元素存储**：支持复杂的海报元素结构和样式
3. **完善的状态管理**：任务全生命周期跟踪
4. **高性能设计**：Redis缓存热数据，Kafka异步处理

## 技术栈

- **服务端**：Go + Gin + GORM + Redis
- **算法端**：Python + LangGraph + FastAPI
- **存储**：MySQL + Redis + MinIO/OSS
- **消息队列**：Kafka
- **容器化**：Docker + Kubernetes