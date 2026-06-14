#!/usr/bin/env bash
# Day 5 测试：二维码 + download zip + token 鉴权
BASE=http://localhost:8787

echo "=== 1. 部署（响应应含 qrCode data URL） ==="
RESP=$(curl -s -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "description": "multi-file test",
    "title": "Multi",
    "files": [
      {"path":"index.html","content":"<h1>hello</h1>"},
      {"path":"styles.css","content":"h1{color:red}"},
      {"path":"script.js","content":"console.log(1)"}
    ]
  }')
echo "$RESP" | python -c "import sys,json; d=json.load(sys.stdin); print('success:', d.get('success')); print('url:', d.get('url')); print('qrCode len:', len(d.get('qrCode',''))); print('qrCode preview:', d.get('qrCode','')[:50])"
CODE=$(echo "$RESP" | python -c "import sys,json; print(json.load(sys.stdin)['code'])")
echo "code: $CODE"
echo

sleep 1.2

echo "=== 2. 部署单 HTML（响应应含 qrCode） ==="
RESP2=$(curl -s -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -d '{"description":"single html","content":"<h1>single</h1>"}')
SINGLE_CODE=$(echo "$RESP2" | python -c "import sys,json; print(json.load(sys.stdin)['code'])")
echo "code: $SINGLE_CODE"
echo "$RESP2" | python -c "import sys,json; d=json.load(sys.stdin); print('qrCode len:', len(d.get('qrCode','')))"
echo

echo "=== 3. download 多文件 site（应返回 zip） ==="
curl -s -o /tmp/test.zip -w "HTTP %{http_code}, Content-Type: %{content_type}, Size: %{size_download}\n" \
  "$BASE/api/deploy/content?code=$CODE&download=1"
echo "zip 文件类型检查:"
file /tmp/test.zip 2>/dev/null || python -c "print(open('/tmp/test.zip','rb').read(4))"  # PK\x03\x04 = zip
echo

echo "=== 4. download 单 HTML site（应直接返回 HTML） ==="
curl -s -w "\nHTTP %{http_code}, Content-Type: %{content_type}\n" \
  "$BASE/api/deploy/content?code=$SINGLE_CODE&download=1"
echo

echo "=== 5. JSON 模式 GetContent ==="
curl -s "$BASE/api/deploy/content?code=$CODE" | python -c "import sys,json; d=json.load(sys.stdin); print('version:', d['version']); print('files:', [f['path'] for f in d['files']])"
echo

echo "=== 6. 创建 token ==="
TOK=$(curl -s -X POST $BASE/api/token \
  -H "Content-Type: application/json" \
  -d '{"label":"test-token","isAdmin":false}')
echo "$TOK" | python -c "import sys,json; d=json.load(sys.stdin); print('id:', d['id']); print('label:', d['label']); print('token len:', len(d['token']))"
TOKEN=$(echo "$TOK" | python -c "import sys,json; print(json.load(sys.stdin)['token'])")
echo

echo "=== 7. 列出 tokens ==="
curl -s $BASE/api/tokens | python -c "import sys,json; d=json.load(sys.stdin); print('count:', len(d['tokens'])); [print(' -', t['id'][:8], t.get('label'), 'admin=',t['isAdmin'],'revoked=',t['isRevoked']) for t in d['tokens']]"
echo

echo "=== 8. 部署时带 token（dev 模式应该忽略 token） ==="
curl -s -X POST $BASE/api/deploy \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"description":"with token","content":"x"}' | python -c "import sys,json; d=json.load(sys.stdin); print('success:', d.get('success'), 'code:', d.get('code'))"
echo

echo "=== 9. 吊销 token ==="
TOK_ID=$(echo "$TOK" | python -c "import sys,json; print(json.load(sys.stdin)['id'])")
curl -s -X DELETE $BASE/api/tokens/$TOK_ID
echo
echo

echo "=== 10. 列 tokens（应该有一个 revoked） ==="
curl -s $BASE/api/tokens | python -c "import sys,json; d=json.load(sys.stdin); [print(' -', t['id'][:8], 'revoked=', t['isRevoked']) for t in d['tokens']]"
