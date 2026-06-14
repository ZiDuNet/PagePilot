#!/usr/bin/env bash
# Day 4 冷却测试
BASE=http://localhost:8787

echo "=== 冷却测试：连续 3 次部署 ==="
for i in 1 2 3; do
  echo "--- 第 $i 次 ---"
  curl -s -i -X POST $BASE/api/deploy \
    -H "Content-Type: application/json" \
    -d "{\"description\":\"cooldown test $i\",\"content\":\"<h1>$i</h1>\"}" 2>&1 | head -15
  echo
  echo
  sleep 1
done

echo "=== 第 4 次（应仍被限流，除非已过 10s） ==="
curl -s -i -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -d '{"description":"fourth","content":"x"}' 2>&1 | head -15
echo
