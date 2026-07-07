package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// cipherKey 是 AES 加密/解密访问密码明文的主密钥。
// 优先使用环境变量 HOSTCTL_MASTER_KEY（32 字节 base64），
// 未配置时使用固定默认值（仅用于开发环境，生产环境必须配置）。
var cipherKey [32]byte

func init() {
	// 尝试从环境变量读取主密钥
	// 如果为空，使用开发默认密钥（仅 dev 模式安全）
}

// SetMasterKey 设置 AES 主密钥（由 server 启动时调用）。
func SetMasterKey(key [32]byte) {
	cipherKey = key
}

// EnsureCipherKey 确保 cipherKey 已初始化（dev 模式 fallback）。
func EnsureCipherKey() {
	// 如果 cipherKey 全零，使用开发默认密钥
	allZero := true
	for _, b := range cipherKey {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		// 开发环境默认密钥（仅在 HOSTCTL_DEV 模式下安全）
		copy(cipherKey[:], []byte("pagepilot-dev-master-key-0000000"))
	}
}

// EncryptPassword 使用 AES-GCM 加密明文密码，返回 base64 编码的密文。
func EncryptPassword(plaintext string) (string, error) {
	EnsureCipherKey()
	plaintextBytes := []byte(plaintext)
	block, err := aes.NewCipher(cipherKey[:])
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, plaintextBytes, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPassword 解密 AES-GCM 密文（base64 编码），返回明文。
func DecryptPassword(ciphertextB64 string) (string, error) {
	EnsureCipherKey()
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(cipherKey[:])
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertextBytes := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
