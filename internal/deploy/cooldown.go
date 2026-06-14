package deploy

import (
	"sync"
	"time"
)

// Cooldown 是一个简单的多维度限流器。
// 三层维度（任一触发即拒绝）：
//  1. 全局：所有部署请求共享
//  2. 按 owner token：同一 token 部署过快
//  3. 按客户端 IP：同一 IP 部署过快（防止 token 共用、绕过维度 2）
//
// 每个维度都是"上次成功时刻 + 冷却时长"。下一次请求必须等到该时刻过后。
// 不用 golang.org/x/time/rate 是因为：
//   - 内部团队规模，简单时间窗足够
//   - rate.Limiter 的 burst 不直观，10 秒"全局冷却"语义清晰
type Cooldown struct {
	window time.Duration

	mu     sync.Mutex
	global time.Time
	byTok  map[string]time.Time
	byIP   map[string]time.Time
}

// NewCooldown 构造。
func NewCooldown(window time.Duration) *Cooldown {
	return &Cooldown{
		window: window,
		byTok:  make(map[string]time.Time),
		byIP:   make(map[string]time.Time),
	}
}

// SetWindow updates the cooldown window without discarding existing buckets.
func (c *Cooldown) SetWindow(window time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.window = window
}

// Check 返回 (ok, retryAfter)。
// ok=false 时，retryAfter > 0 表示需要等待的时间。
// 任一维度未过冷却都会拒绝；retryAfter 取三个维度中最大的等待时间。
func (c *Cooldown) Check(tokenID, clientIP string) (ok bool, retryAfter time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var maxWait time.Duration

	if rem := c.global.Sub(now); rem > 0 {
		maxWait = rem
	}
	if t, has := c.byTok[tokenID]; has {
		if rem := t.Sub(now); rem > maxWait {
			maxWait = rem
		}
	}
	if clientIP != "" {
		if t, has := c.byIP[clientIP]; has {
			if rem := t.Sub(now); rem > maxWait {
				maxWait = rem
			}
		}
	}

	if maxWait > 0 {
		return false, maxWait
	}
	return true, 0
}

// Consume 在一次部署成功后调用，登记本次时间戳到三个维度。
// 失败的请求不消耗冷却（避免污染计数；错误重试不该被进一步限流）。
// 返回下次可部署需要等待的时间窗（用于响应里 nextAvailableAt）。
func (c *Cooldown) Consume(tokenID, clientIP string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	until := time.Now().Add(c.window)
	c.global = until
	if tokenID != "" {
		c.byTok[tokenID] = until
	}
	if clientIP != "" {
		c.byIP[clientIP] = until
	}
	return c.window
}

// Cleanup 删除已过期的条目，避免 map 无限增长。
// 调用方按需周期性触发。
func (c *Cooldown) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, t := range c.byTok {
		if !t.After(now) {
			delete(c.byTok, k)
		}
	}
	for k, t := range c.byIP {
		if !t.After(now) {
			delete(c.byIP, k)
		}
	}
}
