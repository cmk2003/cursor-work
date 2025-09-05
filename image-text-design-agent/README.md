# AI图文设计Agent系统

## 项目概述

本项目旨在构建一个面向小白用户的AI图文设计Agent系统，通过自然语言输入和简单交互，帮助用户在3分钟内生成专业级的图文设计作品。

### 核心特点

- **简单易用**：一句话描述需求即可生成设计
- **智能生成**：基于LangGraph的智能工作流
- **可视化编辑**：直观的画布编辑功能
- **高质量输出**：确保文字正确、排版美观、风格统一

## 项目结构

```
image-text-design-agent/
├── docs/                    # 项目文档
│   ├── architecture/       # 架构设计
│   ├── api/               # API文档
│   ├── database/          # 数据库设计
│   └── deployment/        # 部署方案
├── frontend/              # 前端项目（Vue 3）
│   ├── src/
│   │   ├── components/    # 组件库
│   │   ├── views/        # 页面视图
│   │   ├── utils/        # 工具函数
│   │   ├── api/          # API接口
│   │   └── store/        # 状态管理
│   └── public/           # 静态资源
├── backend/              # 后端服务（Go + Python）
│   ├── services/         # 微服务
│   │   ├── gateway/      # API网关
│   │   ├── user/         # 用户服务
│   │   ├── project/      # 项目服务
│   │   ├── task/         # 任务服务
│   │   └── algorithm/    # 算法调度服务
│   ├── pkg/              # 公共包
│   └── cmd/              # 启动入口
└── algorithm/            # 算法端（Python）
    ├── langgraph/        # LangGraph工作流
    │   ├── nodes/        # 节点定义
    │   ├── graphs/       # 图定义
    │   └── agents/       # Agent实现
    ├── models/           # 模型接口
    └── utils/            # 工具函数
```

## 技术栈

### 前端
- Vue 3 + TypeScript
- Canvas API / Fabric.js
- Element Plus UI组件库
- Pinia状态管理
- Axios HTTP客户端

### 后端
- Go (API网关、业务服务)
- Python (算法调度)
- MySQL (业务数据)
- Redis (缓存、会话)
- Kafka (消息队列)
- Docker + Kubernetes

### 算法
- LangChain + LangGraph
- Stable Diffusion / DALL-E
- OCR (PaddleOCR)
- OpenCV (图像处理)
- FastAPI (算法服务)

## 快速开始

详见各模块的README文档。