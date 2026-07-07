import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");

function cssBlock(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = styles.match(new RegExp(`${escaped}\\s*{([\\s\\S]*?)}`));
  return match?.[1] || "";
}

test("市场详情元信息使用紧凑属性条", () => {
  assert.match(source, /<em>分类<\/em><strong>{marketCategoryLabel\(item\.category\)}<\/strong>/);
  assert.match(source, /<em>类型<\/em><strong>{marketFileType\(item\)}<\/strong>/);
  assert.match(source, /<em>赞<\/em><strong>{compactNumber\(item\.likeCount \|\| 0\)}<\/strong>/);
  assert.match(source, /<em>访问<\/em><strong>{compactNumber\(item\.viewCount \|\| 0\)}<\/strong>/);
  assert.match(styles, /\.market-detail-meta\.compact\s*{[\s\S]*display:\s*flex/);
  assert.match(styles, /\.market-detail-meta\.compact span\s*{[\s\S]*min-height:\s*28px/);
});

test("市场详情右侧卡片不产生独立滚动条", () => {
  const card = cssBlock(".market-detail-card");
  const versions = cssBlock(".detail-version-list");
  assert.match(card, /overflow:\s*visible/);
  assert.doesNotMatch(card, /overflow:\s*auto/);
  assert.doesNotMatch(card, /max-height:/);
  assert.match(versions, /overflow:\s*visible/);
  assert.doesNotMatch(versions, /overflow:\s*auto/);
});

test("加密作品详情页也使用后端访问页渲染密码输入", () => {
  const detail = source.slice(
    source.indexOf("function MarketDetailViewFull"),
    source.indexOf("function MarketUseDrawer")
  );
  assert.doesNotMatch(detail, /isLocked\s*\?\s*\(/, "加密详情预览不应被前端静态锁定占位拦截");
  assert.doesNotMatch(detail, />网页已加密</, "详情页不应只显示静态加密提示");
  assert.match(detail, /className="detail-access-form"/, "加密详情页应显示访问密码表单");
  assert.match(detail, /\/api\/deploys\/\$\{encodeURIComponent\(item\.code\)\}\/access/, "密码表单应调用站点访问验证接口");
  assert.match(detail, /detailPreviewReady\s*\?\s*\(/, "详情预览应继续延迟挂载 iframe");
  assert.match(detail, /src=\{withPreviewParam\(appURL\)\}/, "iframe 应请求后端应用页，由后端决定直开或显示密码输入");
});
