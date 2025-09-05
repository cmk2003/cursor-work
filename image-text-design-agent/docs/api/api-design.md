# API接口设计文档

## 1. API设计原则

### 1.1 RESTful规范
- 使用HTTP动词表达操作：GET（查询）、POST（创建）、PUT（更新）、DELETE（删除）
- 使用名词表示资源，使用复数形式
- 使用HTTP状态码表示结果
- 支持过滤、排序、分页

### 1.2 版本管理
- URL路径版本：`/api/v1/`
- 主版本号变更表示不兼容的API修改
- 次版本号变更表示向后兼容的功能新增

### 1.3 认证授权
- 使用JWT进行身份认证
- Bearer Token放在Authorization头部
- 支持API Key认证（第三方接入）

### 1.4 响应格式
```json
{
  "code": 0,          // 业务状态码，0表示成功
  "message": "success", // 状态描述
  "data": {},         // 响应数据
  "timestamp": 1234567890, // 时间戳
  "request_id": "uuid"     // 请求ID，用于追踪
}
```

## 2. 认证接口

### 2.1 用户注册

**接口地址**：`POST /api/v1/auth/register`

**请求参数**：
```json
{
  "username": "string",     // 用户名，3-20个字符
  "password": "string",     // 密码，8-32个字符
  "email": "string",        // 邮箱
  "captcha": "string",      // 验证码
  "invite_code": "string"   // 邀请码（可选）
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "注册成功",
  "data": {
    "user_id": "123456",
    "username": "testuser",
    "email": "test@example.com",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

### 2.2 用户登录

**接口地址**：`POST /api/v1/auth/login`

**请求参数**：
```json
{
  "username": "string",  // 用户名或邮箱
  "password": "string",  // 密码
  "captcha": "string"    // 验证码（多次失败后需要）
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "登录成功",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 7200,
    "user": {
      "user_id": "123456",
      "username": "testuser",
      "email": "test@example.com",
      "avatar": "https://..."
    }
  }
}
```

### 2.3 刷新Token

**接口地址**：`POST /api/v1/auth/refresh`

**请求参数**：
```json
{
  "refresh_token": "string"
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "刷新成功",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 7200
  }
}
```

## 3. 用户接口

### 3.1 获取用户信息

**接口地址**：`GET /api/v1/users/profile`

**请求头**：
```
Authorization: Bearer {token}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "user_id": "123456",
    "username": "testuser",
    "email": "test@example.com",
    "avatar": "https://...",
    "created_at": "2024-01-01T00:00:00Z",
    "stats": {
      "project_count": 10,
      "design_count": 50,
      "storage_used": 1024000000,
      "storage_limit": 5368709120
    }
  }
}
```

### 3.2 更新用户信息

**接口地址**：`PUT /api/v1/users/profile`

**请求参数**：
```json
{
  "nickname": "string",
  "avatar": "string",
  "bio": "string"
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "更新成功",
  "data": {
    "user_id": "123456",
    "nickname": "新昵称",
    "avatar": "https://...",
    "bio": "个人简介"
  }
}
```

## 4. 项目管理接口

### 4.1 创建项目

**接口地址**：`POST /api/v1/projects`

**请求参数**：
```json
{
  "name": "string",        // 项目名称
  "description": "string", // 项目描述
  "type": "string",        // 项目类型：poster/banner/social/custom
  "tags": ["string"],      // 标签
  "is_public": false       // 是否公开
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "创建成功",
  "data": {
    "project_id": "proj_123456",
    "name": "双十一促销海报",
    "type": "poster",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

### 4.2 获取项目列表

**接口地址**：`GET /api/v1/projects`

**请求参数**：
```
page: 1              // 页码
page_size: 20        // 每页数量
keyword: string      // 搜索关键词
type: string         // 项目类型
sort: created_at     // 排序字段
order: desc          // 排序方向
```

**响应示例**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "project_id": "proj_123456",
        "name": "双十一促销海报",
        "type": "poster",
        "preview": "https://...",
        "design_count": 5,
        "updated_at": "2024-01-01T00:00:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 100,
      "total_pages": 5
    }
  }
}
```

### 4.3 获取项目详情

**接口地址**：`GET /api/v1/projects/{project_id}`

**响应示例**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "project_id": "proj_123456",
    "name": "双十一促销海报",
    "description": "电商促销活动海报设计",
    "type": "poster",
    "tags": ["促销", "电商"],
    "designs": [
      {
        "design_id": "design_123",
        "version": 1,
        "preview": "https://...",
        "created_at": "2024-01-01T00:00:00Z"
      }
    ],
    "collaborators": [
      {
        "user_id": "123",
        "username": "designer1",
        "role": "editor"
      }
    ]
  }
}
```

## 5. 设计生成接口

### 5.1 创建生成任务

**接口地址**：`POST /api/v1/designs/generate`

**请求参数**：
```json
{
  "project_id": "string",     // 项目ID
  "prompt": "string",         // 设计描述
  "style": "string",          // 风格：business/cartoon/minimalist等
  "size": {                   // 尺寸
    "width": 1024,
    "height": 1024
  },
  "advanced_options": {       // 高级选项
    "color_scheme": ["#FF0000", "#00FF00"], // 配色方案
    "font_style": "modern",   // 字体风格
    "layout": "centered",     // 布局方式
    "elements": [             // 指定元素
      {
        "type": "text",
        "content": "限时优惠",
        "position": "top"
      }
    ]
  },
  "reference_images": ["url"] // 参考图片
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "任务创建成功",
  "data": {
    "task_id": "task_123456",
    "status": "pending",
    "estimated_time": 30,
    "queue_position": 5
  }
}
```

### 5.2 查询生成状态

**接口地址**：`GET /api/v1/designs/tasks/{task_id}/status`

**响应示例**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "task_123456",
    "status": "processing",   // pending/processing/completed/failed
    "progress": {
      "phase": "generating_image",
      "percentage": 60,
      "message": "正在生成图像..."
    },
    "result": null,           // 完成时包含结果
    "error": null             // 失败时包含错误信息
  }
}
```

### 5.3 获取生成结果

**接口地址**：`GET /api/v1/designs/{design_id}`

**响应示例**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "design_id": "design_123456",
    "project_id": "proj_123",
    "task_id": "task_123",
    "images": [
      {
        "url": "https://...",
        "width": 1024,
        "height": 1024,
        "format": "png"
      }
    ],
    "metadata": {
      "prompt": "生成一个蓝色商务风格海报",
      "style": "business",
      "generation_time": 25.3,
      "model_version": "sd_xl_1.0"
    },
    "editable_elements": [
      {
        "id": "elem_1",
        "type": "text",
        "content": "限时优惠",
        "position": {"x": 100, "y": 50},
        "style": {
          "font": "Arial",
          "size": 48,
          "color": "#FFFFFF"
        }
      }
    ]
  }
}
```

## 6. 画布编辑接口

### 6.1 保存画布状态

**接口地址**：`POST /api/v1/canvas/save`

**请求参数**：
```json
{
  "design_id": "string",
  "canvas_data": {
    "version": "5.3.0",
    "objects": [
      {
        "type": "text",
        "text": "标题文字",
        "left": 100,
        "top": 100,
        "fontSize": 30,
        "fill": "#000000"
      }
    ],
    "background": "#FFFFFF",
    "width": 1024,
    "height": 768
  },
  "preview": "base64..."      // 预览图
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "保存成功",
  "data": {
    "design_id": "design_123456",
    "version": 2,
    "saved_at": "2024-01-01T00:00:00Z"
  }
}
```

### 6.2 导出设计

**接口地址**：`POST /api/v1/canvas/export`

**请求参数**：
```json
{
  "design_id": "string",
  "format": "png",        // png/jpg/pdf/svg
  "quality": 100,         // 质量（jpg）
  "scale": 2,             // 缩放倍数
  "include_bleed": false  // 是否包含出血
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "导出成功",
  "data": {
    "export_id": "export_123",
    "download_url": "https://...",
    "expires_at": "2024-01-01T01:00:00Z",
    "file_size": 2048000,
    "format": "png"
  }
}
```

## 7. 灵感广场接口

### 7.1 获取推荐作品

**接口地址**：`GET /api/v1/gallery/featured`

**请求参数**：
```
category: string     // 分类：all/poster/banner/social
style: string        // 风格标签
page: 1
page_size: 20
```

**响应示例**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "work_id": "work_123",
        "title": "圣诞节促销海报",
        "preview": "https://...",
        "author": {
          "user_id": "123",
          "username": "designer1",
          "avatar": "https://..."
        },
        "stats": {
          "views": 1000,
          "likes": 100,
          "uses": 50
        },
        "tags": ["圣诞", "促销", "红色"]
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 1000
    }
  }
}
```

### 7.2 使用模板

**接口地址**：`POST /api/v1/gallery/{work_id}/use`

**请求参数**：
```json
{
  "project_id": "string",    // 目标项目ID
  "customization": {         // 定制选项
    "texts": {
      "title": "自定义标题",
      "subtitle": "自定义副标题"
    },
    "colors": {
      "primary": "#FF0000",
      "secondary": "#00FF00"
    }
  }
}
```

**响应示例**：
```json
{
  "code": 0,
  "message": "模板应用成功",
  "data": {
    "design_id": "design_new_123",
    "project_id": "proj_123",
    "preview": "https://..."
  }
}
```

## 8. WebSocket接口

### 8.1 连接建立

**地址**：`ws://api.example.com/ws`

**认证**：
```javascript
const ws = new WebSocket('ws://api.example.com/ws', {
  headers: {
    'Authorization': 'Bearer ' + token
  }
});
```

### 8.2 任务进度推送

**消息格式**：
```json
{
  "type": "task_progress",
  "data": {
    "task_id": "task_123456",
    "status": "processing",
    "progress": {
      "phase": "generating_image",
      "percentage": 60,
      "message": "正在生成图像..."
    }
  }
}
```

### 8.3 协作编辑

**消息格式**：
```json
{
  "type": "canvas_update",
  "data": {
    "design_id": "design_123",
    "user_id": "user_456",
    "operation": {
      "type": "add",
      "object": {
        "type": "text",
        "text": "新文字",
        "left": 100,
        "top": 100
      }
    }
  }
}
```

## 9. 错误码定义

### 9.1 系统级错误码

| 错误码 | 说明 | HTTP状态码 |
|-------|------|-----------|
| 0 | 成功 | 200 |
| 10001 | 参数错误 | 400 |
| 10002 | 认证失败 | 401 |
| 10003 | 权限不足 | 403 |
| 10004 | 资源不存在 | 404 |
| 10005 | 请求方法不允许 | 405 |
| 10006 | 请求超时 | 408 |
| 10007 | 请求频率限制 | 429 |
| 10008 | 服务器内部错误 | 500 |
| 10009 | 服务不可用 | 503 |

### 9.2 业务错误码

| 错误码 | 说明 | 模块 |
|-------|------|-----|
| 20001 | 用户名已存在 | 认证 |
| 20002 | 密码错误 | 认证 |
| 20003 | 验证码错误 | 认证 |
| 20004 | Token已过期 | 认证 |
| 30001 | 项目数量超限 | 项目 |
| 30002 | 项目名称重复 | 项目 |
| 30003 | 无项目访问权限 | 项目 |
| 40001 | 生成任务队列已满 | 设计 |
| 40002 | 不支持的图片格式 | 设计 |
| 40003 | 图片尺寸超限 | 设计 |
| 40004 | 生成失败 | 设计 |

## 10. API限流策略

### 10.1 限流规则

| API类型 | 限流策略 | 说明 |
|--------|---------|------|
| 认证接口 | 5次/分钟 | 防止暴力破解 |
| 查询接口 | 100次/分钟 | 常规查询限制 |
| 生成接口 | 10次/小时 | 资源密集型操作 |
| 导出接口 | 20次/小时 | 防止资源滥用 |

### 10.2 限流响应头

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
```

## 11. SDK示例

### 11.1 JavaScript SDK

```javascript
// 初始化
const client = new DesignClient({
  apiKey: 'your-api-key',
  baseURL: 'https://api.example.com'
});

// 生成设计
const task = await client.designs.generate({
  prompt: '创建一个蓝色商务风格海报',
  style: 'business',
  size: { width: 1024, height: 1024 }
});

// 监听进度
task.on('progress', (progress) => {
  console.log(`进度: ${progress.percentage}%`);
});

// 获取结果
const result = await task.wait();
console.log('生成完成:', result);
```

### 11.2 Python SDK

```python
from design_client import DesignClient

# 初始化
client = DesignClient(
    api_key='your-api-key',
    base_url='https://api.example.com'
)

# 生成设计
task = client.designs.generate(
    prompt='创建一个蓝色商务风格海报',
    style='business',
    size={'width': 1024, 'height': 1024}
)

# 等待完成
result = task.wait_for_completion()
print(f'生成完成: {result}')
```