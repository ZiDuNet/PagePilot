#!/usr/bin/env bash
# Day 3 端到端测试：版本管理（append / list / lock / switch / overwrite / status / delete）
set -uo pipefail
BASE=http://localhost:8787

echo "=== 准备：部署 v1（customCode=mytest） ==="
curl -s -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "description": "v1 initial",
    "title": "My Site",
    "enableCustomCode": true,
    "customCode": "mytest",
    "files": [
      {"path":"index.html","content":"<h1>v1</h1>"},
      {"path":"styles.css","content":"h1{color:red}"}
    ]
  }'
echo
echo

# dev 冷却 1 秒，部署之间留 1.2s 等冷却
sleep 1.2

echo "=== 1. 追加 v2（createVersion=true） ==="
curl -s -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "description": "v2 second",
    "enableCustomCode": true,
    "customCode": "mytest",
    "createVersion": true,
    "files": [{"path":"index.html","content":"<h1>v2</h1>"}]
  }'
echo
echo

echo "=== 2. 列出版本 ==="
curl -s $BASE/api/deploys/mytest/versions
echo
echo

echo "=== 3. 锁定 v1 ==="
curl -s -X POST $BASE/api/deploys/mytest/versions/1/lock \
  -H "Content-Type: application/json" \
  -d '{"locked": true}'
echo
echo

echo "=== 4. 尝试覆盖锁定的 v1（应失败：VERSION_LOCKED） ==="
curl -s -X PATCH $BASE/api/deploys/mytest/versions/1 \
  -H "Content-Type: application/json" \
  -d '{"description":"hack","content":"<h1>hacked</h1>"}'
echo
echo

echo "=== 5. 覆盖未锁定的 v2（应成功） ==="
curl -s -X PATCH $BASE/api/deploys/mytest/versions/2 \
  -H "Content-Type: application/json" \
  -d '{"description":"v2 overwritten","content":"<h1>v2-new</h1>"}'
echo
echo

echo "=== 6. 切换当前版本到 v1 ==="
curl -s -X PATCH $BASE/api/deploys/mytest/current \
  -H "Content-Type: application/json" \
  -d '{"versionNumber": 1}'
echo
echo

echo "=== 7. 设置 v2 状态为 inactive ==="
curl -s -X PATCH $BASE/api/deploys/mytest/versions/2 \
  -H "Content-Type: application/json" \
  -d '{"status": "inactive"}'
echo
echo

echo "=== 8. 尝试切换到 inactive 的 v2（应失败） ==="
curl -s -X PATCH $BASE/api/deploys/mytest/current \
  -H "Content-Type: application/json" \
  -d '{"versionNumber": 2}'
echo
echo

echo "=== 9. 切换回 v2 并激活 ==="
curl -s -X PATCH $BASE/api/deploys/mytest/versions/2 \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'
curl -s -X PATCH $BASE/api/deploys/mytest/current \
  -H "Content-Type: application/json" \
  -d '{"versionNumber": 2}'
echo
echo

echo "=== 10. 列出版本（验证 current 应为 v2） ==="
curl -s $BASE/api/deploys/mytest/versions
echo
echo

echo "=== 11. 删除 v1（应失败：锁定） ==="
curl -s -X DELETE $BASE/api/deploys/mytest/versions/1
echo
echo

echo "=== 12. 解锁 v1 ==="
curl -s -X POST $BASE/api/deploys/mytest/versions/1/lock \
  -H "Content-Type: application/json" \
  -d '{"locked": false}'
echo
echo

echo "=== 13. 删除 v1（应成功） ==="
curl -s -X DELETE $BASE/api/deploys/mytest/versions/1
echo
echo

echo "=== 14. GetContent 当前版本 ==="
curl -s "$BASE/api/deploy/content?code=mytest"
echo
echo

echo "=== 15. GetContent v=2 ==="
curl -s "$BASE/api/deploy/content?code=mytest&version=2"
echo
echo

echo "=== 16. 访问 current 静态文件 ==="
curl -s $BASE/mytest
echo
echo

echo "=== 17. 错误场景：切换到不存在的版本 ==="
curl -s -X PATCH $BASE/api/deploys/mytest/current \
  -H "Content-Type: application/json" \
  -d '{"versionNumber": 99}'
echo
echo

echo "=== 18. 错误场景：列出不存在的 code ==="
curl -s $BASE/api/deploys/nonexistent/versions
echo
echo

echo "=== 19. 按 UUID 切换 ==="
# 先拿到 v2 的 UUID
VID=$(curl -s $BASE/api/deploys/mytest/versions | python -c "import sys,json; d=json.load(sys.stdin); print([v for v in d['versions'] if v['versionNumber']==2][0]['id'])" 2>/dev/null)
echo "v2 UUID: $VID"
curl -s -X PATCH $BASE/api/deploys/mytest/current \
  -H "Content-Type: application/json" \
  -d "{\"versionId\": \"$VID\"}"
echo
echo
