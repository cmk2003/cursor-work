# 部署和运维方案

## 1. 部署架构概览

### 1.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        用户访问层                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   CloudFlare │  │   阿里云CDN  │  │    WAF       │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                        负载均衡层                            │
│  ┌────────────────────────────────────────────────────┐    │
│  │              Nginx / ALB (Application Load Balancer)│    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes集群                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │  前端Pod    │  │  API网关Pod  │  │ 业务服务Pod │        │
│  │  (3 replicas)│  │ (3 replicas) │  │ (3+ replicas)│       │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ 算法服务Pod │  │   GPU节点    │  │ 监控Agent   │        │
│  │ (Auto-scale)│  │  (AI推理)    │  │  (DaemonSet)│        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                        数据存储层                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │MySQL主从集群│  │Redis Cluster │  │   MinIO     │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
│  ┌─────────────┐  ┌─────────────┐                          │
│  │   Kafka     │  │Elasticsearch│                          │
│  └─────────────┘  └─────────────┘                          │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 环境划分

| 环境 | 用途 | 配置规模 | 域名 |
|-----|------|---------|------|
| 开发环境 | 开发测试 | 最小配置 | dev.example.com |
| 测试环境 | 功能测试 | 中等配置 | test.example.com |
| 预发布环境 | 上线前验证 | 生产配置 | staging.example.com |
| 生产环境 | 对外服务 | 高可用配置 | api.example.com |

## 2. Kubernetes部署配置

### 2.1 命名空间规划

```yaml
# namespaces.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: design-agent-prod
  labels:
    env: production
---
apiVersion: v1
kind: Namespace
metadata:
  name: design-agent-staging
  labels:
    env: staging
---
apiVersion: v1
kind: Namespace
metadata:
  name: design-agent-monitoring
  labels:
    purpose: monitoring
```

### 2.2 前端部署

```yaml
# frontend-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: design-agent-prod
spec:
  replicas: 3
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
      containers:
      - name: frontend
        image: registry.example.com/design-agent/frontend:latest
        ports:
        - containerPort: 80
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 80
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 80
          initialDelaySeconds: 5
          periodSeconds: 5
        env:
        - name: API_URL
          value: "https://api.example.com"
        - name: CDN_URL
          value: "https://cdn.example.com"
---
apiVersion: v1
kind: Service
metadata:
  name: frontend-service
  namespace: design-agent-prod
spec:
  selector:
    app: frontend
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: ClusterIP
```

### 2.3 API网关部署

```yaml
# gateway-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-gateway
  namespace: design-agent-prod
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api-gateway
  template:
    metadata:
      labels:
        app: api-gateway
    spec:
      containers:
      - name: gateway
        image: registry.example.com/design-agent/gateway:latest
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
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
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: jwt-secret
              key: secret
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: api-gateway-service
  namespace: design-agent-prod
spec:
  selector:
    app: api-gateway
  ports:
  - protocol: TCP
    port: 8080
    targetPort: 8080
  type: ClusterIP
```

### 2.4 算法服务部署（带GPU）

```yaml
# algorithm-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: algorithm-service
  namespace: design-agent-prod
spec:
  replicas: 2
  selector:
    matchLabels:
      app: algorithm-service
  template:
    metadata:
      labels:
        app: algorithm-service
    spec:
      nodeSelector:
        gpu: "true"
      containers:
      - name: algorithm
        image: registry.example.com/design-agent/algorithm:latest
        ports:
        - containerPort: 8000
        resources:
          requests:
            memory: "8Gi"
            cpu: "2000m"
            nvidia.com/gpu: 1
          limits:
            memory: "16Gi"
            cpu: "4000m"
            nvidia.com/gpu: 1
        volumeMounts:
        - name: model-cache
          mountPath: /models
        - name: shm
          mountPath: /dev/shm
        env:
        - name: CUDA_VISIBLE_DEVICES
          value: "0"
        - name: MODEL_CACHE_DIR
          value: "/models"
        - name: TRITON_SERVER_URL
          value: "triton-inference:8001"
      volumes:
      - name: model-cache
        persistentVolumeClaim:
          claimName: model-cache-pvc
      - name: shm
        emptyDir:
          medium: Memory
          sizeLimit: 2Gi
```

### 2.5 水平自动扩缩容（HPA）

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-gateway-hpa
  namespace: design-agent-prod
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-gateway
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: algorithm-hpa
  namespace: design-agent-prod
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: algorithm-service
  minReplicas: 2
  maxReplicas: 5
  metrics:
  - type: Pods
    pods:
      metric:
        name: pending_tasks
      target:
        type: AverageValue
        averageValue: "30"
```

## 3. 数据库部署

### 3.1 MySQL主从配置

```yaml
# mysql-master.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql-master
  namespace: design-agent-prod
spec:
  serviceName: mysql-master
  replicas: 1
  selector:
    matchLabels:
      app: mysql-master
  template:
    metadata:
      labels:
        app: mysql-master
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        ports:
        - containerPort: 3306
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mysql-secret
              key: root-password
        - name: MYSQL_DATABASE
          value: design_agent
        volumeMounts:
        - name: mysql-data
          mountPath: /var/lib/mysql
        - name: mysql-config
          mountPath: /etc/mysql/conf.d
        resources:
          requests:
            memory: "2Gi"
            cpu: "1000m"
          limits:
            memory: "4Gi"
            cpu: "2000m"
      volumes:
      - name: mysql-config
        configMap:
          name: mysql-master-config
  volumeClaimTemplates:
  - metadata:
      name: mysql-data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: "fast-ssd"
      resources:
        requests:
          storage: 100Gi
```

### 3.2 Redis集群配置

```yaml
# redis-cluster.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-cluster
  namespace: design-agent-prod
spec:
  serviceName: redis-cluster
  replicas: 6
  selector:
    matchLabels:
      app: redis-cluster
  template:
    metadata:
      labels:
        app: redis-cluster
    spec:
      containers:
      - name: redis
        image: redis:7.0-alpine
        command: ["redis-server"]
        args: ["/conf/redis.conf"]
        ports:
        - containerPort: 6379
        - containerPort: 16379
        volumeMounts:
        - name: conf
          mountPath: /conf
        - name: data
          mountPath: /data
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
      volumes:
      - name: conf
        configMap:
          name: redis-cluster-config
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: "standard"
      resources:
        requests:
          storage: 10Gi
```

## 4. CI/CD流程

### 4.1 GitLab CI配置

```yaml
# .gitlab-ci.yml
stages:
  - test
  - build
  - deploy

variables:
  DOCKER_REGISTRY: registry.example.com
  KUBE_NAMESPACE_DEV: design-agent-dev
  KUBE_NAMESPACE_PROD: design-agent-prod

# 测试阶段
test:frontend:
  stage: test
  image: node:18
  script:
    - cd frontend
    - npm install
    - npm run test
    - npm run lint
  only:
    - merge_requests
    - develop
    - main

test:backend:
  stage: test
  image: golang:1.21
  script:
    - cd backend
    - go mod download
    - go test ./...
    - golangci-lint run
  only:
    - merge_requests
    - develop
    - main

test:algorithm:
  stage: test
  image: python:3.11
  script:
    - cd algorithm
    - pip install -r requirements.txt
    - pytest tests/
    - flake8 .
  only:
    - merge_requests
    - develop
    - main

# 构建阶段
build:frontend:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  script:
    - docker build -t $DOCKER_REGISTRY/design-agent/frontend:$CI_COMMIT_SHA frontend/
    - docker push $DOCKER_REGISTRY/design-agent/frontend:$CI_COMMIT_SHA
    - docker tag $DOCKER_REGISTRY/design-agent/frontend:$CI_COMMIT_SHA $DOCKER_REGISTRY/design-agent/frontend:latest
    - docker push $DOCKER_REGISTRY/design-agent/frontend:latest
  only:
    - develop
    - main

build:backend:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  script:
    - docker build -t $DOCKER_REGISTRY/design-agent/gateway:$CI_COMMIT_SHA backend/services/gateway/
    - docker push $DOCKER_REGISTRY/design-agent/gateway:$CI_COMMIT_SHA
  only:
    - develop
    - main

# 部署阶段
deploy:dev:
  stage: deploy
  image: bitnami/kubectl:latest
  script:
    - kubectl set image deployment/frontend frontend=$DOCKER_REGISTRY/design-agent/frontend:$CI_COMMIT_SHA -n $KUBE_NAMESPACE_DEV
    - kubectl set image deployment/api-gateway gateway=$DOCKER_REGISTRY/design-agent/gateway:$CI_COMMIT_SHA -n $KUBE_NAMESPACE_DEV
    - kubectl rollout status deployment/frontend -n $KUBE_NAMESPACE_DEV
    - kubectl rollout status deployment/api-gateway -n $KUBE_NAMESPACE_DEV
  only:
    - develop
  environment:
    name: development
    url: https://dev.example.com

deploy:prod:
  stage: deploy
  image: bitnami/kubectl:latest
  script:
    - kubectl set image deployment/frontend frontend=$DOCKER_REGISTRY/design-agent/frontend:$CI_COMMIT_SHA -n $KUBE_NAMESPACE_PROD
    - kubectl set image deployment/api-gateway gateway=$DOCKER_REGISTRY/design-agent/gateway:$CI_COMMIT_SHA -n $KUBE_NAMESPACE_PROD
    - kubectl rollout status deployment/frontend -n $KUBE_NAMESPACE_PROD
    - kubectl rollout status deployment/api-gateway -n $KUBE_NAMESPACE_PROD
  only:
    - main
  when: manual
  environment:
    name: production
    url: https://api.example.com
```

### 4.2 Dockerfile示例

```dockerfile
# frontend/Dockerfile
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

# backend/services/gateway/Dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/gateway .
EXPOSE 8080
CMD ["./gateway"]

# algorithm/Dockerfile
FROM pytorch/pytorch:2.1.0-cuda12.1-cudnn8-runtime
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE 8000
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
```

## 5. 监控和日志

### 5.1 Prometheus配置

```yaml
# prometheus-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: design-agent-monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s
    
    scrape_configs:
    - job_name: 'kubernetes-pods'
      kubernetes_sd_configs:
      - role: pod
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__
    
    - job_name: 'node-exporter'
      kubernetes_sd_configs:
      - role: node
      relabel_configs:
      - source_labels: [__address__]
        regex: '(.*):10250'
        replacement: '${1}:9100'
        target_label: __address__
```

### 5.2 Grafana Dashboard配置

```json
{
  "dashboard": {
    "title": "Design Agent Monitoring",
    "panels": [
      {
        "title": "API Request Rate",
        "targets": [
          {
            "expr": "sum(rate(http_requests_total[5m])) by (endpoint)"
          }
        ]
      },
      {
        "title": "Generation Task Queue",
        "targets": [
          {
            "expr": "generation_tasks_pending"
          }
        ]
      },
      {
        "title": "GPU Utilization",
        "targets": [
          {
            "expr": "nvidia_gpu_utilization"
          }
        ]
      },
      {
        "title": "Error Rate",
        "targets": [
          {
            "expr": "sum(rate(http_requests_total{status=~'5..'}[5m]))"
          }
        ]
      }
    ]
  }
}
```

### 5.3 ELK Stack配置

```yaml
# elasticsearch.yaml
apiVersion: elasticsearch.k8s.elastic.co/v1
kind: Elasticsearch
metadata:
  name: elasticsearch
  namespace: design-agent-monitoring
spec:
  version: 8.11.0
  nodeSets:
  - name: default
    count: 3
    config:
      node.store.allow_mmap: false
    volumeClaimTemplates:
    - metadata:
        name: elasticsearch-data
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 100Gi
        storageClassName: standard

# logstash-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: logstash-config
  namespace: design-agent-monitoring
data:
  logstash.conf: |
    input {
      beats {
        port => 5044
      }
    }
    
    filter {
      if [kubernetes][labels][app] == "api-gateway" {
        grok {
          match => { "message" => "%{TIMESTAMP_ISO8601:timestamp} %{LOGLEVEL:level} %{GREEDYDATA:message}" }
        }
      }
      
      if [kubernetes][labels][app] == "algorithm-service" {
        json {
          source => "message"
        }
      }
    }
    
    output {
      elasticsearch {
        hosts => ["elasticsearch:9200"]
        index => "design-agent-%{+YYYY.MM.dd}"
      }
    }
```

## 6. 安全配置

### 6.1 网络策略

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: api-gateway-policy
  namespace: design-agent-prod
spec:
  podSelector:
    matchLabels:
      app: api-gateway
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: nginx-ingress
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: user-service
    - podSelector:
        matchLabels:
          app: project-service
    - podSelector:
        matchLabels:
          app: task-service
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 3306  # MySQL
    - protocol: TCP
      port: 6379  # Redis
```

### 6.2 RBAC配置

```yaml
# rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: design-agent-role
  namespace: design-agent-prod
rules:
- apiGroups: [""]
  resources: ["pods", "services", "configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets"]
  verbs: ["get", "list", "watch", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: design-agent-rolebinding
  namespace: design-agent-prod
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: design-agent-role
subjects:
- kind: ServiceAccount
  name: design-agent-sa
  namespace: design-agent-prod
```

### 6.3 Secret管理

```bash
# 创建数据库密码
kubectl create secret generic db-secret \
  --from-literal=host=mysql-master.design-agent-prod.svc.cluster.local \
  --from-literal=username=design_agent \
  --from-literal=password='your-secure-password' \
  -n design-agent-prod

# 创建JWT密钥
kubectl create secret generic jwt-secret \
  --from-literal=secret='your-jwt-secret-key' \
  -n design-agent-prod

# 创建镜像拉取密钥
kubectl create secret docker-registry regcred \
  --docker-server=registry.example.com \
  --docker-username=your-username \
  --docker-password=your-password \
  --docker-email=your-email \
  -n design-agent-prod
```

## 7. 备份和恢复

### 7.1 数据备份策略

```yaml
# backup-cronjob.yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: mysql-backup
  namespace: design-agent-prod
spec:
  schedule: "0 2 * * *"  # 每天凌晨2点
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: mysql-backup
            image: mysql:8.0
            command:
            - /bin/bash
            - -c
            - |
              DATE=$(date +%Y%m%d_%H%M%S)
              mysqldump -h mysql-master -u root -p$MYSQL_ROOT_PASSWORD \
                --all-databases --single-transaction --routines --triggers \
                | gzip > /backup/mysql_backup_${DATE}.sql.gz
              
              # 上传到对象存储
              mc config host add minio https://minio.example.com $MINIO_ACCESS_KEY $MINIO_SECRET_KEY
              mc cp /backup/mysql_backup_${DATE}.sql.gz minio/backups/mysql/
              
              # 删除7天前的本地备份
              find /backup -name "*.sql.gz" -mtime +7 -delete
            env:
            - name: MYSQL_ROOT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: root-password
            volumeMounts:
            - name: backup
              mountPath: /backup
          volumes:
          - name: backup
            persistentVolumeClaim:
              claimName: backup-pvc
          restartPolicy: OnFailure
```

### 7.2 灾难恢复流程

```bash
#!/bin/bash
# disaster-recovery.sh

# 1. 恢复MySQL数据
kubectl exec -it mysql-master-0 -n design-agent-prod -- bash -c "
  gunzip < /backup/mysql_backup_latest.sql.gz | mysql -u root -p\$MYSQL_ROOT_PASSWORD
"

# 2. 恢复Redis数据
kubectl exec -it redis-cluster-0 -n design-agent-prod -- redis-cli --cluster restore

# 3. 重启所有服务
kubectl rollout restart deployment -n design-agent-prod

# 4. 验证服务状态
kubectl get pods -n design-agent-prod
kubectl logs -f deployment/api-gateway -n design-agent-prod
```

## 8. 性能优化

### 8.1 资源优化建议

```yaml
# resource-quotas.yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: design-agent-quota
  namespace: design-agent-prod
spec:
  hard:
    requests.cpu: "100"
    requests.memory: "200Gi"
    limits.cpu: "200"
    limits.memory: "400Gi"
    persistentvolumeclaims: "20"
    services.loadbalancers: "5"
```

### 8.2 性能调优参数

```yaml
# performance-tuning.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: performance-config
  namespace: design-agent-prod
data:
  nginx.conf: |
    worker_processes auto;
    worker_rlimit_nofile 65535;
    
    events {
        worker_connections 4096;
        use epoll;
        multi_accept on;
    }
    
    http {
        keepalive_timeout 65;
        keepalive_requests 100;
        
        gzip on;
        gzip_vary on;
        gzip_proxied any;
        gzip_comp_level 6;
        gzip_types text/plain text/css text/xml text/javascript application/json application/javascript;
    }
  
  mysql.cnf: |
    [mysqld]
    innodb_buffer_pool_size = 2G
    innodb_log_file_size = 256M
    innodb_flush_log_at_trx_commit = 2
    innodb_flush_method = O_DIRECT
    max_connections = 500
    query_cache_size = 256M
    query_cache_type = 1
  
  redis.conf: |
    maxmemory 2gb
    maxmemory-policy allkeys-lru
    save ""
    appendonly yes
    appendfsync everysec
```

## 9. 运维脚本

### 9.1 健康检查脚本

```bash
#!/bin/bash
# health-check.sh

NAMESPACE="design-agent-prod"
SERVICES=("frontend" "api-gateway" "user-service" "project-service" "task-service" "algorithm-service")

echo "=== 服务健康状态检查 ==="
echo "时间: $(date)"
echo ""

# 检查Pod状态
echo "### Pod状态 ###"
kubectl get pods -n $NAMESPACE

# 检查服务端点
echo -e "\n### 服务端点 ###"
for service in "${SERVICES[@]}"; do
    endpoints=$(kubectl get endpoints $service-service -n $NAMESPACE -o jsonpath='{.subsets[0].addresses[*].ip}' 2>/dev/null)
    if [ -z "$endpoints" ]; then
        echo "❌ $service: 无可用端点"
    else
        echo "✅ $service: $endpoints"
    fi
done

# 检查数据库连接
echo -e "\n### 数据库状态 ###"
kubectl exec -it mysql-master-0 -n $NAMESPACE -- mysqladmin -u root -p$MYSQL_ROOT_PASSWORD status

# 检查Redis状态
echo -e "\n### Redis状态 ###"
kubectl exec -it redis-cluster-0 -n $NAMESPACE -- redis-cli ping

# 检查API响应
echo -e "\n### API健康检查 ###"
API_URL="https://api.example.com/health"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" $API_URL)
if [ "$HTTP_CODE" = "200" ]; then
    echo "✅ API网关: 正常 (HTTP $HTTP_CODE)"
else
    echo "❌ API网关: 异常 (HTTP $HTTP_CODE)"
fi
```

### 9.2 日志收集脚本

```bash
#!/bin/bash
# collect-logs.sh

NAMESPACE="design-agent-prod"
OUTPUT_DIR="/tmp/design-agent-logs-$(date +%Y%m%d-%H%M%S)"
mkdir -p $OUTPUT_DIR

echo "收集日志到: $OUTPUT_DIR"

# 收集所有Pod日志
for pod in $(kubectl get pods -n $NAMESPACE -o jsonpath='{.items[*].metadata.name}'); do
    echo "收集 $pod 的日志..."
    kubectl logs $pod -n $NAMESPACE --all-containers=true > "$OUTPUT_DIR/$pod.log"
done

# 收集系统事件
kubectl get events -n $NAMESPACE > "$OUTPUT_DIR/events.log"

# 打包日志
tar -czf "$OUTPUT_DIR.tar.gz" -C /tmp $(basename $OUTPUT_DIR)
echo "日志已打包: $OUTPUT_DIR.tar.gz"
```

## 10. 故障处理手册

### 10.1 常见问题处理

#### Pod无法启动
```bash
# 查看Pod详情
kubectl describe pod <pod-name> -n design-agent-prod

# 查看Pod日志
kubectl logs <pod-name> -n design-agent-prod --previous

# 常见原因：
# 1. 镜像拉取失败 - 检查镜像仓库凭据
# 2. 资源不足 - 检查节点资源
# 3. 配置错误 - 检查ConfigMap和Secret
```

#### 数据库连接失败
```bash
# 测试数据库连接
kubectl run -it --rm mysql-client --image=mysql:8.0 --restart=Never -- \
  mysql -h mysql-master.design-agent-prod.svc.cluster.local -u root -p

# 检查网络策略
kubectl get networkpolicy -n design-agent-prod

# 检查DNS解析
kubectl run -it --rm busybox --image=busybox --restart=Never -- \
  nslookup mysql-master.design-agent-prod.svc.cluster.local
```

#### 服务503错误
```bash
# 检查服务端点
kubectl get endpoints -n design-agent-prod

# 检查HPA状态
kubectl get hpa -n design-agent-prod

# 查看负载均衡配置
kubectl describe ingress -n design-agent-prod
```

### 10.2 紧急回滚流程

```bash
#!/bin/bash
# emergency-rollback.sh

SERVICE=$1
NAMESPACE="design-agent-prod"

if [ -z "$SERVICE" ]; then
    echo "Usage: ./emergency-rollback.sh <service-name>"
    exit 1
fi

echo "开始回滚 $SERVICE..."

# 获取上一个版本
PREVIOUS_REVISION=$(kubectl rollout history deployment/$SERVICE -n $NAMESPACE | tail -2 | head -1 | awk '{print $1}')

# 执行回滚
kubectl rollout undo deployment/$SERVICE --to-revision=$PREVIOUS_REVISION -n $NAMESPACE

# 等待回滚完成
kubectl rollout status deployment/$SERVICE -n $NAMESPACE

echo "回滚完成!"
```

## 11. 成本优化

### 11.1 资源利用率监控

```yaml
# cost-optimization.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cost-alerts
  namespace: design-agent-monitoring
data:
  alerts.yml: |
    groups:
    - name: cost_alerts
      rules:
      - alert: LowCPUUtilization
        expr: avg(rate(container_cpu_usage_seconds_total[5m])) by (pod) < 0.1
        for: 30m
        annotations:
          summary: "Pod {{ $labels.pod }} CPU利用率过低"
          
      - alert: LowMemoryUtilization
        expr: avg(container_memory_usage_bytes / container_spec_memory_limit_bytes) by (pod) < 0.2
        for: 30m
        annotations:
          summary: "Pod {{ $labels.pod }} 内存利用率过低"
```

### 11.2 自动化成本优化

```bash
#!/bin/bash
# optimize-resources.sh

# 分析资源使用情况
kubectl top pods -n design-agent-prod --sort-by=cpu
kubectl top pods -n design-agent-prod --sort-by=memory

# 推荐资源调整
echo "### 资源优化建议 ###"
kubectl get pods -n design-agent-prod -o json | jq -r '
  .items[] | 
  select(.status.phase=="Running") | 
  {
    name: .metadata.name,
    cpu_request: .spec.containers[0].resources.requests.cpu,
    cpu_limit: .spec.containers[0].resources.limits.cpu,
    memory_request: .spec.containers[0].resources.requests.memory,
    memory_limit: .spec.containers[0].resources.limits.memory
  }'
```