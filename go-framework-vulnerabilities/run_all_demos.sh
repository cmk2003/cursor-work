#!/bin/bash

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Go框架漏洞演示启动脚本 ===${NC}\n"

# 检查Go环境
if ! command -v go &> /dev/null; then
    echo -e "${RED}错误: 未找到Go环境，请先安装Go${NC}"
    exit 1
fi

# 下载依赖
echo -e "${YELLOW}下载依赖...${NC}"
go mod download

# 创建必要的目录
mkdir -p uploads

# 启动各个演示
echo -e "\n${GREEN}启动漏洞演示服务...${NC}\n"

# 1. 路径遍历和权限绕过演示
echo -e "${BLUE}[1/3] 启动Gin路径遍历和权限绕过演示${NC}"
echo "服务将运行在 http://localhost:8080 (漏洞版) 和 http://localhost:8081 (安全版)"
go run gin-vuln/path_traversal.go &
PID1=$!

sleep 2

# 2. 竞态条件演示
echo -e "\n${BLUE}[2/3] 启动Gin竞态条件演示${NC}"
echo "服务将运行在 http://localhost:8082 (漏洞版) 和 http://localhost:8083 (安全版)"
go run gin-vuln/race_condition.go &
PID2=$!

sleep 2

# 3. 时间盲注演示
echo -e "\n${BLUE}[3/3] 启动XORM时间盲注演示${NC}"
echo "服务将运行在 http://localhost:8084"
go run xorm-vuln/time_based_blind_injection.go &
PID3=$!

sleep 2

# 4. 二阶SQL注入演示（非服务器模式）
echo -e "\n${BLUE}[4/4] 运行XORM二阶SQL注入演示${NC}"
go run xorm-vuln/second_order_injection.go

echo -e "\n${GREEN}所有演示服务已启动！${NC}"
echo -e "\n${YELLOW}测试方法：${NC}"
echo "1. 运行测试脚本: ./test_vulnerabilities.sh"
echo "2. 查看详细分析: cat docs/vulnerability_analysis.md"
echo "3. 手动测试各个漏洞（参考每个服务的输出）"

echo -e "\n${YELLOW}按 Ctrl+C 停止所有服务${NC}"

# 等待用户中断
trap "kill $PID1 $PID2 $PID3 2>/dev/null; echo -e '\n${RED}所有服务已停止${NC}'; exit" INT

wait