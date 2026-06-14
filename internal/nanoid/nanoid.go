// Package nanoid 提供简短的随机短码生成，URL 友好。
// 自己实现，避免引外部依赖。
package nanoid

import (
	"crypto/rand"
	"fmt"
)

// 默认字母表（OpenAPI 兼容：仅小写字母 + 数字 + -）
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// Generate 生成 size 字符长度的随机串。
func Generate(size int) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, size)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

// Default 生成默认 6 字符短码。
func Default() (string, error) {
	return Generate(6)
}
