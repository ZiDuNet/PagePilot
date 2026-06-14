#!/usr/bin/env bash
# Day 7 端到端：admin UI 配置（baseURL 热更新） + 站点列表 + 删除 + MCP server
# 前置：hostctl-server --dev 在 http://localhost:8787 跑着
set -uo pipefail
BASE=http://localhost:8787

echo "===== 1. 健康检查 ====="
curl -s $BASE/api/health && echo
echo

echo "===== 2. 读取初始配置 ====="
curl -s $BASE/api/config && echo
echo

echo "===== 3. 热修改 baseURL ====="
curl -s -X PUT $BASE/api/config \
  -H "Content-Type: application/json" \
  -d '{"publicBaseURL":"https://changed.example.com"}'
echo
echo

echo "===== 4. 验证 baseURL 已生效（重启也会持久化） ====="
curl -s $BASE/api/config | python -c "import sys,json;d=json.load(sys.stdin);print('current publicBaseURL:', d['publicBaseURL'])"
echo

echo "===== 5. 错误的 baseURL 应被拒 ====="
curl -s -X PUT $BASE/api/config \
  -H "Content-Type: application/json" \
  -d '{"publicBaseURL":"not-a-url"}'
echo
echo

echo "===== 6. 部署新 site（应使用新的 baseURL） ====="
sleep 1.2  # dev 冷却
DEPLOY=$(curl -s -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "description":"day7 test",
    "enableCustomCode":true,
    "customCode":"day7",
    "files":[{"path":"index.html","content":"<h1>day7</h1>"}]
  }')
echo "$DEPLOY"
echo
URL=$(echo "$DEPLOY" | python -c "import sys,json;print(json.load(sys.stdin)['url'])")
echo "部署返回的 URL: $URL"
if [[ "$URL" == https://changed.example.com/day7 ]]; then
  echo "✓ baseURL 已被采用"
else
  echo "✗ baseURL 未被采用"
fi
echo

echo "===== 7. 列出所有站点 ====="
curl -s $BASE/api/admin/sites | python -m json.tool
echo

echo "===== 8. MCP server: tools/list ====="
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  HOSTCTL_SERVER=$BASE ./bin/hostctl-mcp.exe 2>&1 | python -c "import sys,json; d=json.load(sys.stdin); print('Tools:', [t['name'] for t in d['result']['tools']])"
echo

echo "===== 9. MCP server: list_versions ====="
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_versions","arguments":{"code":"day7"}}}' | \
  HOSTCTL_SERVER=$BASE ./bin/hostctl-mcp.exe 2>&1 | python -m json.tool
echo

echo "===== 10. MCP server: lock_version ====="
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lock_version","arguments":{"code":"day7","version":1,"locked":true}}}' | \
  HOSTCTL_SERVER=$BASE ./bin/hostctl-mcp.exe 2>&1 | python -m json.tool
echo

echo "===== 11. 恢复 baseURL ====="
curl -s -X PUT $BASE/api/config \
  -H "Content-Type: application/json" \
  -d '{"publicBaseURL":"http://localhost:8787"}'
echo
echo

echo "===== 12. 删除测试 site ====="
curl -s -X DELETE $BASE/api/admin/sites/day7 && echo
echo

echo "===== 13. 验证站点已删除 ====="
curl -s $BASE/api/admin/sites | python -c "import sys,json;print('剩余站点:', [s['code'] for s in json.load(sys.stdin)['sites']])"
echo

echo "===== Done ====="
