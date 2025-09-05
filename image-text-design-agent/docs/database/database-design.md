# 数据库设计文档

## 1. 数据库选型

### 1.1 主数据库
- **MySQL 8.0**：关系型数据，存储核心业务数据
- 支持事务、外键约束、索引优化
- 主从复制实现读写分离

### 1.2 缓存数据库
- **Redis 7.0**：缓存热点数据、会话管理、任务队列
- 支持数据持久化、主从复制、哨兵模式

### 1.3 搜索引擎
- **Elasticsearch 8.0**：全文搜索、日志分析
- 支持中文分词、聚合分析

### 1.4 对象存储
- **MinIO/阿里云OSS**：存储图片、设计文件
- 支持版本管理、CDN加速

## 2. 数据库架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        应用层                                │
├─────────────────────────────────────────────────────────────┤
│                    数据访问层（DAO）                         │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │  主库(写)   │  │  从库(读)   │  │    Redis    │        │
│  │  MySQL-Master│  │ MySQL-Slave │  │   Cluster   │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
│  ┌─────────────┐  ┌─────────────┐                          │
│  │Elasticsearch│  │   MinIO     │                          │
│  │   Cluster   │  │   Cluster   │                          │
│  └─────────────┘  └─────────────┘                          │
└─────────────────────────────────────────────────────────────┘
```

## 3. 核心数据表设计

### 3.1 用户模块

#### users（用户表）
```sql
CREATE TABLE `users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '用户ID',
  `username` VARCHAR(50) NOT NULL COMMENT '用户名',
  `email` VARCHAR(100) NOT NULL COMMENT '邮箱',
  `password` VARCHAR(255) NOT NULL COMMENT '密码哈希',
  `nickname` VARCHAR(50) DEFAULT NULL COMMENT '昵称',
  `avatar` VARCHAR(500) DEFAULT NULL COMMENT '头像URL',
  `bio` TEXT DEFAULT NULL COMMENT '个人简介',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态：0-禁用 1-正常',
  `role` VARCHAR(20) NOT NULL DEFAULT 'user' COMMENT '角色：user/vip/admin',
  `last_login_at` TIMESTAMP NULL DEFAULT NULL COMMENT '最后登录时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `deleted_at` TIMESTAMP NULL DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  UNIQUE KEY `uk_email` (`email`),
  KEY `idx_status` (`status`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';
```

#### user_auth_logs（认证日志表）
```sql
CREATE TABLE `user_auth_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '用户ID',
  `action` VARCHAR(20) NOT NULL COMMENT '操作类型：login/logout/register',
  `ip` VARCHAR(45) NOT NULL COMMENT 'IP地址',
  `user_agent` VARCHAR(500) DEFAULT NULL COMMENT '用户代理',
  `status` TINYINT NOT NULL COMMENT '状态：0-失败 1-成功',
  `reason` VARCHAR(100) DEFAULT NULL COMMENT '失败原因',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_action` (`action`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户认证日志';
```

#### user_settings（用户设置表）
```sql
CREATE TABLE `user_settings` (
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
  `language` VARCHAR(10) NOT NULL DEFAULT 'zh-CN' COMMENT '语言',
  `theme` VARCHAR(20) NOT NULL DEFAULT 'light' COMMENT '主题',
  `email_notification` TINYINT NOT NULL DEFAULT 1 COMMENT '邮件通知',
  `privacy_mode` TINYINT NOT NULL DEFAULT 0 COMMENT '隐私模式',
  `storage_limit` BIGINT NOT NULL DEFAULT 5368709120 COMMENT '存储限制(字节)',
  `storage_used` BIGINT NOT NULL DEFAULT 0 COMMENT '已用存储(字节)',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户设置';
```

### 3.2 项目模块

#### projects（项目表）
```sql
CREATE TABLE `projects` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '项目ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '创建者ID',
  `name` VARCHAR(100) NOT NULL COMMENT '项目名称',
  `description` TEXT DEFAULT NULL COMMENT '项目描述',
  `type` VARCHAR(20) NOT NULL COMMENT '项目类型：poster/banner/social/custom',
  `cover` VARCHAR(500) DEFAULT NULL COMMENT '封面图URL',
  `is_public` TINYINT NOT NULL DEFAULT 0 COMMENT '是否公开',
  `is_template` TINYINT NOT NULL DEFAULT 0 COMMENT '是否模板',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态：0-删除 1-正常',
  `design_count` INT NOT NULL DEFAULT 0 COMMENT '设计数量',
  `view_count` INT NOT NULL DEFAULT 0 COMMENT '浏览次数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_type` (`type`),
  KEY `idx_is_public` (`is_public`),
  KEY `idx_created_at` (`created_at`),
  FULLTEXT KEY `ft_name_desc` (`name`, `description`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='项目表';
```

#### project_collaborators（项目协作者表）
```sql
CREATE TABLE `project_collaborators` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `project_id` BIGINT UNSIGNED NOT NULL COMMENT '项目ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
  `role` VARCHAR(20) NOT NULL COMMENT '角色：viewer/editor/admin',
  `invited_by` BIGINT UNSIGNED NOT NULL COMMENT '邀请人ID',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_user` (`project_id`, `user_id`),
  KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='项目协作者';
```

#### project_tags（项目标签表）
```sql
CREATE TABLE `project_tags` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `project_id` BIGINT UNSIGNED NOT NULL COMMENT '项目ID',
  `tag` VARCHAR(50) NOT NULL COMMENT '标签',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_project_id` (`project_id`),
  KEY `idx_tag` (`tag`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='项目标签';
```

### 3.3 设计模块

#### designs（设计表）
```sql
CREATE TABLE `designs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '设计ID',
  `project_id` BIGINT UNSIGNED NOT NULL COMMENT '项目ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '创建者ID',
  `task_id` VARCHAR(100) DEFAULT NULL COMMENT '生成任务ID',
  `name` VARCHAR(100) DEFAULT NULL COMMENT '设计名称',
  `version` INT NOT NULL DEFAULT 1 COMMENT '版本号',
  `canvas_data` JSON DEFAULT NULL COMMENT '画布数据',
  `preview_url` VARCHAR(500) DEFAULT NULL COMMENT '预览图URL',
  `width` INT NOT NULL DEFAULT 1024 COMMENT '宽度',
  `height` INT NOT NULL DEFAULT 1024 COMMENT '高度',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态：0-删除 1-正常',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_project_id` (`project_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_task_id` (`task_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='设计表';
```

#### design_history（设计历史表）
```sql
CREATE TABLE `design_history` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `design_id` BIGINT UNSIGNED NOT NULL COMMENT '设计ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '操作者ID',
  `version` INT NOT NULL COMMENT '版本号',
  `operation` VARCHAR(50) NOT NULL COMMENT '操作类型',
  `canvas_data` JSON DEFAULT NULL COMMENT '画布数据快照',
  `preview_url` VARCHAR(500) DEFAULT NULL COMMENT '预览图URL',
  `description` VARCHAR(200) DEFAULT NULL COMMENT '操作描述',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_design_id` (`design_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='设计历史';
```

### 3.4 任务模块

#### generation_tasks（生成任务表）
```sql
CREATE TABLE `generation_tasks` (
  `id` VARCHAR(100) NOT NULL COMMENT '任务ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
  `project_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '项目ID',
  `type` VARCHAR(20) NOT NULL COMMENT '任务类型：image_generation/style_transfer',
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT '状态：pending/processing/completed/failed',
  `params` JSON NOT NULL COMMENT '任务参数',
  `result` JSON DEFAULT NULL COMMENT '任务结果',
  `progress` JSON DEFAULT NULL COMMENT '进度信息',
  `error` TEXT DEFAULT NULL COMMENT '错误信息',
  `started_at` TIMESTAMP NULL DEFAULT NULL COMMENT '开始时间',
  `completed_at` TIMESTAMP NULL DEFAULT NULL COMMENT '完成时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_project_id` (`project_id`),
  KEY `idx_status` (`status`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='生成任务';
```

#### task_queue（任务队列表）
```sql
CREATE TABLE `task_queue` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id` VARCHAR(100) NOT NULL COMMENT '任务ID',
  `priority` INT NOT NULL DEFAULT 0 COMMENT '优先级',
  `retry_count` INT NOT NULL DEFAULT 0 COMMENT '重试次数',
  `max_retries` INT NOT NULL DEFAULT 3 COMMENT '最大重试次数',
  `scheduled_at` TIMESTAMP NULL DEFAULT NULL COMMENT '计划执行时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_id` (`task_id`),
  KEY `idx_priority_scheduled` (`priority` DESC, `scheduled_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务队列';
```

### 3.5 素材模块

#### materials（素材表）
```sql
CREATE TABLE `materials` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '素材ID',
  `user_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '上传者ID(NULL表示系统素材)',
  `category` VARCHAR(50) NOT NULL COMMENT '分类：image/icon/font/template',
  `name` VARCHAR(100) NOT NULL COMMENT '素材名称',
  `description` TEXT DEFAULT NULL COMMENT '素材描述',
  `file_url` VARCHAR(500) NOT NULL COMMENT '文件URL',
  `thumbnail_url` VARCHAR(500) DEFAULT NULL COMMENT '缩略图URL',
  `file_size` BIGINT NOT NULL COMMENT '文件大小(字节)',
  `mime_type` VARCHAR(50) NOT NULL COMMENT 'MIME类型',
  `metadata` JSON DEFAULT NULL COMMENT '元数据',
  `tags` JSON DEFAULT NULL COMMENT '标签',
  `is_public` TINYINT NOT NULL DEFAULT 0 COMMENT '是否公开',
  `usage_count` INT NOT NULL DEFAULT 0 COMMENT '使用次数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_category` (`category`),
  KEY `idx_is_public` (`is_public`),
  KEY `idx_created_at` (`created_at`),
  FULLTEXT KEY `ft_name_desc` (`name`, `description`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='素材库';
```

### 3.6 灵感广场模块

#### gallery_works（作品表）
```sql
CREATE TABLE `gallery_works` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '作品ID',
  `design_id` BIGINT UNSIGNED NOT NULL COMMENT '设计ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '作者ID',
  `title` VARCHAR(200) NOT NULL COMMENT '作品标题',
  `description` TEXT DEFAULT NULL COMMENT '作品描述',
  `category` VARCHAR(50) NOT NULL COMMENT '分类',
  `tags` JSON DEFAULT NULL COMMENT '标签',
  `preview_url` VARCHAR(500) NOT NULL COMMENT '预览图',
  `is_featured` TINYINT NOT NULL DEFAULT 0 COMMENT '是否精选',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '状态：0-下架 1-正常',
  `view_count` INT NOT NULL DEFAULT 0 COMMENT '浏览数',
  `like_count` INT NOT NULL DEFAULT 0 COMMENT '点赞数',
  `use_count` INT NOT NULL DEFAULT 0 COMMENT '使用数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_design_id` (`design_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_category` (`category`),
  KEY `idx_is_featured` (`is_featured`),
  KEY `idx_created_at` (`created_at`),
  FULLTEXT KEY `ft_title_desc` (`title`, `description`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='灵感广场作品';
```

#### gallery_interactions（互动记录表）
```sql
CREATE TABLE `gallery_interactions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `work_id` BIGINT UNSIGNED NOT NULL COMMENT '作品ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
  `type` VARCHAR(20) NOT NULL COMMENT '类型：view/like/collect/use',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_work_user_type` (`work_id`, `user_id`, `type`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_type` (`type`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='作品互动记录';
```

## 4. 索引优化策略

### 4.1 查询优化索引

```sql
-- 用户登录查询优化
ALTER TABLE users ADD INDEX idx_username_password (username, password);

-- 项目列表查询优化
ALTER TABLE projects ADD INDEX idx_user_status_created (user_id, status, created_at DESC);

-- 任务状态查询优化
ALTER TABLE generation_tasks ADD INDEX idx_user_status_created (user_id, status, created_at DESC);

-- 素材搜索优化
ALTER TABLE materials ADD INDEX idx_category_public_created (category, is_public, created_at DESC);
```

### 4.2 全文搜索优化

```sql
-- 中文全文搜索配置
ALTER TABLE projects ADD FULLTEXT ft_name_desc (name, description) WITH PARSER ngram;
ALTER TABLE gallery_works ADD FULLTEXT ft_title_desc (title, description) WITH PARSER ngram;
ALTER TABLE materials ADD FULLTEXT ft_name_desc (name, description) WITH PARSER ngram;
```

## 5. 分表策略

### 5.1 水平分表

对于大表采用水平分表策略：

#### user_auth_logs 按月分表
```sql
-- 2024年1月日志表
CREATE TABLE `user_auth_logs_202401` LIKE `user_auth_logs`;

-- 2024年2月日志表
CREATE TABLE `user_auth_logs_202402` LIKE `user_auth_logs`;
```

#### generation_tasks 按用户ID取模分表
```sql
-- 分10个表
CREATE TABLE `generation_tasks_0` LIKE `generation_tasks`;
CREATE TABLE `generation_tasks_1` LIKE `generation_tasks`;
-- ... 到 generation_tasks_9
```

### 5.2 分表路由规则

```go
// 根据用户ID路由到对应的任务表
func GetTaskTableName(userId int64) string {
    tableIndex := userId % 10
    return fmt.Sprintf("generation_tasks_%d", tableIndex)
}

// 根据时间路由到对应的日志表
func GetLogTableName(date time.Time) string {
    return fmt.Sprintf("user_auth_logs_%s", date.Format("200601"))
}
```

## 6. 数据库备份策略

### 6.1 备份方案

1. **全量备份**：每天凌晨2点执行
2. **增量备份**：每小时执行
3. **实时备份**：通过binlog实时同步到备份库

### 6.2 备份脚本

```bash
#!/bin/bash
# 全量备份脚本

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/data/backup/mysql"
DB_NAME="design_agent"

# 创建备份目录
mkdir -p ${BACKUP_DIR}

# 执行备份
mysqldump \
  --host=${DB_HOST} \
  --port=${DB_PORT} \
  --user=${DB_USER} \
  --password=${DB_PASS} \
  --single-transaction \
  --routines \
  --triggers \
  --databases ${DB_NAME} | gzip > ${BACKUP_DIR}/${DB_NAME}_${DATE}.sql.gz

# 删除7天前的备份
find ${BACKUP_DIR} -name "*.sql.gz" -mtime +7 -delete
```

## 7. 性能优化建议

### 7.1 查询优化

1. **使用覆盖索引**
```sql
-- 优化前
SELECT * FROM users WHERE username = 'test';

-- 优化后
SELECT id, username, email FROM users WHERE username = 'test';
```

2. **避免SELECT ***
```sql
-- 只查询需要的字段
SELECT id, name, preview_url FROM projects WHERE user_id = 123;
```

3. **使用EXPLAIN分析查询**
```sql
EXPLAIN SELECT * FROM designs WHERE project_id = 123 ORDER BY created_at DESC LIMIT 10;
```

### 7.2 写入优化

1. **批量插入**
```sql
INSERT INTO project_tags (project_id, tag) VALUES 
(1, 'tag1'), (1, 'tag2'), (1, 'tag3');
```

2. **延迟索引更新**
```sql
ALTER TABLE designs DISABLE KEYS;
-- 批量插入数据
ALTER TABLE designs ENABLE KEYS;
```

### 7.3 缓存策略

1. **热点数据缓存**
```python
# Redis缓存策略
def get_user_info(user_id):
    # 先查缓存
    cache_key = f"user:{user_id}"
    cached = redis.get(cache_key)
    if cached:
        return json.loads(cached)
    
    # 查数据库
    user = db.query("SELECT * FROM users WHERE id = %s", user_id)
    
    # 写入缓存
    redis.setex(cache_key, 3600, json.dumps(user))
    return user
```

2. **查询结果缓存**
```python
# 缓存项目列表
def get_user_projects(user_id, page=1):
    cache_key = f"user_projects:{user_id}:{page}"
    cached = redis.get(cache_key)
    if cached:
        return json.loads(cached)
    
    projects = db.query("""
        SELECT * FROM projects 
        WHERE user_id = %s AND status = 1 
        ORDER BY created_at DESC 
        LIMIT %s, 20
    """, user_id, (page-1)*20)
    
    redis.setex(cache_key, 300, json.dumps(projects))
    return projects
```

## 8. 监控指标

### 8.1 关键指标

1. **查询性能**
   - 慢查询数量
   - 平均查询时间
   - 查询命中率

2. **连接池状态**
   - 活跃连接数
   - 等待连接数
   - 连接使用率

3. **存储空间**
   - 表空间使用率
   - 索引空间占用
   - binlog空间占用

### 8.2 监控SQL

```sql
-- 查看慢查询
SELECT * FROM mysql.slow_log ORDER BY query_time DESC LIMIT 10;

-- 查看表大小
SELECT 
    table_name,
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS size_mb
FROM information_schema.tables
WHERE table_schema = 'design_agent'
ORDER BY size_mb DESC;

-- 查看索引使用情况
SELECT 
    t.table_name,
    s.index_name,
    s.cardinality,
    t.table_rows
FROM information_schema.statistics s
JOIN information_schema.tables t 
    ON s.table_schema = t.table_schema 
    AND s.table_name = t.table_name
WHERE s.table_schema = 'design_agent'
ORDER BY s.cardinality DESC;
```