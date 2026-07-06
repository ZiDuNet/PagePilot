// Package auth 提供 token 鉴权。
//
// 设计：
//   - 部署/管理操作要求 Bearer token
//   - token 明文只在创建时返回一次（response.token），DB 只存 hash
//   - hash 用 SHA-256（够用且性能好；不引 bcrypt 依赖）
//     注：如果 token 是高熵随机串（32+ 字节），SHA-256 完全够。
//     bcrypt/sodium 是为了抵抗暴力穷举弱口令；我们的 token 是加密随机的，无此问题。
//   - GET /api/deploy/content 等读取端点不强制 token（site URL 是公开的）
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

// Errors
var (
	ErrMissing   = errors.New("authorization header missing")
	ErrMalformed = errors.New("authorization header malformed")
	ErrUnknown   = errors.New("token not recognized")
	ErrRevoked   = errors.New("token revoked")
	ErrInvalid   = errors.New("invalid credentials")
	ErrExpired   = errors.New("session expired")
)

// Service 是鉴权服务。
type Service struct {
	store store.Store
}

// New 构造。
func New(s store.Store) *Service {
	return &Service{store: s}
}

// GeneratedToken 是创建 token 的返回值。Plaintext 只在此返回一次。
type GeneratedToken struct {
	ID          string
	Plaintext   string // 32 字节 base64url
	Label       string
	IsAdmin     bool
	OwnerUserID string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
}

type LoginResult struct {
	User      store.AdminUser
	Session   store.AdminSession
	Plaintext string
}

// Generate 创建一个新 token 并写库。返回明文（仅一次可见）。
func (a *Service) Generate(ctx context.Context, label string, isAdmin bool, ownerUserID string, expiresAt *time.Time) (*GeneratedToken, error) {
	// 32 字节随机 → base64url ≈ 43 字符
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrInvalid
	}
	if expiresAt != nil {
		ea := expiresAt.UTC()
		if !ea.After(time.Now().UTC()) {
			return nil, ErrExpired
		}
		expiresAt = &ea
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate token randomness: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := HashToken(plaintext)

	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := a.store.CreateToken(ctx, store.Token{
		ID:          id,
		TokenHash:   hash,
		Label:       label,
		IsAdmin:     isAdmin,
		IsRevoked:   false,
		OwnerUserID: ownerUserID,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}); err != nil {
		return nil, fmt.Errorf("store token: %w", err)
	}

	return &GeneratedToken{
		ID:          id,
		Plaintext:   plaintext,
		Label:       label,
		IsAdmin:     isAdmin,
		OwnerUserID: ownerUserID,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}, nil
}

func (a *Service) EnsureBootstrapAdmin(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil
	}
	n, err := a.store.CountAdminUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	id, err := newUUID()
	if err != nil {
		return err
	}
	return a.store.CreateAdminUser(ctx, store.AdminUser{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		IsAdmin:      true,
		IsActive:     true,
		CanLike:      true,
		DeployLimit:  -1,
		CreatedAt:    time.Now().UTC(),
	})
}

func (a *Service) HasAdminUser(ctx context.Context) (bool, error) {
	n, err := a.store.CountAdminUsers(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (a *Service) CreateFirstAdmin(ctx context.Context, username, password string) (store.AdminUser, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return store.AdminUser{}, ErrInvalid
	}
	n, err := a.store.CountAdminUsers(ctx)
	if err != nil {
		return store.AdminUser{}, err
	}
	if n > 0 {
		return store.AdminUser{}, ErrInvalid
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.AdminUser{}, err
	}
	id, err := newUUID()
	if err != nil {
		return store.AdminUser{}, err
	}
	user := store.AdminUser{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		IsAdmin:      true,
		IsActive:     true,
		CanLike:      true,
		DeployLimit:  -1,
		CreatedAt:    time.Now().UTC(),
	}
	if err := a.store.CreateAdminUser(ctx, user); err != nil {
		return store.AdminUser{}, err
	}
	return user, nil
}

func (a *Service) LoginAdmin(ctx context.Context, username, password string, ttl time.Duration) (*LoginResult, error) {
	user, err := a.store.GetAdminUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalid
		}
		return nil, err
	}
	if !user.IsActive || !VerifyPassword(password, user.PasswordHash) {
		return nil, ErrInvalid
	}
	plaintext, err := randomBase64(32)
	if err != nil {
		return nil, err
	}
	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	session := store.AdminSession{
		ID:          id,
		UserID:      user.ID,
		SessionHash: HashToken(plaintext),
		CreatedAt:   now,
		LastUsedAt:  now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := a.store.CreateAdminSession(ctx, session); err != nil {
		return nil, err
	}
	_ = a.store.TouchAdminUserLastLogin(context.Background(), user.ID)
	return &LoginResult{User: user, Session: session, Plaintext: plaintext}, nil
}

func (a *Service) VerifyAdminSession(ctx context.Context, plaintext string) (store.AdminUser, store.AdminSession, error) {
	if strings.TrimSpace(plaintext) == "" {
		return store.AdminUser{}, store.AdminSession{}, ErrMissing
	}
	session, err := a.store.GetAdminSessionByHash(ctx, HashToken(plaintext))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.AdminUser{}, store.AdminSession{}, ErrUnknown
		}
		return store.AdminUser{}, store.AdminSession{}, err
	}
	if session.RevokedAt != nil {
		return store.AdminUser{}, store.AdminSession{}, ErrRevoked
	}
	if time.Now().After(session.ExpiresAt) {
		return store.AdminUser{}, store.AdminSession{}, ErrExpired
	}
	user, err := a.store.GetAdminUserByID(ctx, session.UserID)
	if err != nil {
		return store.AdminUser{}, store.AdminSession{}, err
	}
	if !user.IsActive {
		return store.AdminUser{}, store.AdminSession{}, ErrRevoked
	}
	go func() {
		_ = a.store.TouchAdminSessionLastUsed(context.Background(), session.ID)
	}()
	return user, session, nil
}

func (a *Service) RevokeAdminSession(ctx context.Context, plaintext string) error {
	if strings.TrimSpace(plaintext) == "" {
		return nil
	}
	session, err := a.store.GetAdminSessionByHash(ctx, HashToken(plaintext))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	return a.store.RevokeAdminSession(ctx, session.ID)
}

func (a *Service) ListUsers(ctx context.Context) ([]store.AdminUser, error) {
	return a.store.ListAdminUsers(ctx)
}

func (a *Service) GetUser(ctx context.Context, id string) (store.AdminUser, error) {
	return a.store.GetAdminUserByID(ctx, id)
}

func (a *Service) CreateUser(ctx context.Context, username, password string, isAdmin bool, deployLimit int) (store.AdminUser, error) {
	return a.CreateUserWithEmail(ctx, username, "", false, password, isAdmin, deployLimit)
}

func (a *Service) CreateUserWithEmail(ctx context.Context, username, email string, emailVerified bool, password string, isAdmin bool, deployLimit int) (store.AdminUser, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	if username == "" || password == "" {
		return store.AdminUser{}, ErrInvalid
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.AdminUser{}, err
	}
	id, err := newUUID()
	if err != nil {
		return store.AdminUser{}, err
	}
	if isAdmin && deployLimit == 0 {
		deployLimit = -1
	}
	user := store.AdminUser{
		ID:            id,
		Username:      username,
		Email:         email,
		EmailVerified: emailVerified,
		PasswordHash:  hash,
		IsAdmin:       isAdmin,
		IsActive:      true,
		CanLike:       true,
		DeployLimit:   deployLimit,
		CreatedAt:     time.Now().UTC(),
	}
	if err := a.store.CreateAdminUser(ctx, user); err != nil {
		return store.AdminUser{}, err
	}
	return user, nil
}

func (a *Service) UpdateUser(ctx context.Context, user store.AdminUser) error {
	if user.IsAdmin && user.DeployLimit == 0 {
		user.DeployLimit = -1
	}
	return a.store.UpdateAdminUser(ctx, user)
}

func (a *Service) DeleteUser(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalid
	}
	return a.store.DeleteAdminUser(ctx, id)
}

func (a *Service) ChangeUserPassword(ctx context.Context, id, oldPassword, newPassword string) error {
	if strings.TrimSpace(id) == "" || oldPassword == "" || newPassword == "" {
		return ErrInvalid
	}
	user, err := a.store.GetAdminUserByID(ctx, id)
	if err != nil {
		return err
	}
	if !user.IsActive || !VerifyPassword(oldPassword, user.PasswordHash) {
		return ErrInvalid
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	return a.store.UpdateAdminUserPassword(ctx, user.ID, hash)
}

func (a *Service) IncrementUserDeployCount(ctx context.Context, id string) (store.AdminUser, error) {
	return a.store.IncrementAdminUserDeployCount(ctx, id)
}

// VerifyToken validates a Bearer header and returns the token record.
func (a *Service) VerifyToken(ctx context.Context, bearerHeader string) (store.Token, error) {
	if bearerHeader == "" {
		return store.Token{}, ErrMissing
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(bearerHeader, prefix) {
		return store.Token{}, ErrMalformed
	}
	plaintext := strings.TrimSpace(strings.TrimPrefix(bearerHeader, prefix))
	if plaintext == "" {
		return store.Token{}, ErrMalformed
	}

	hash := HashToken(plaintext)
	tok, err := a.store.GetTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.Token{}, ErrUnknown
		}
		return store.Token{}, fmt.Errorf("lookup token: %w", err)
	}
	if tok.IsRevoked {
		return store.Token{}, ErrRevoked
	}
	if strings.TrimSpace(tok.OwnerUserID) == "" {
		return store.Token{}, ErrUnknown
	}
	if tok.ExpiresAt != nil && !tok.ExpiresAt.After(time.Now()) {
		return store.Token{}, ErrExpired
	}
	// constant-time compare（虽然 DB lookup 已经唯一，但加一道）
	if subtle.ConstantTimeCompare([]byte(tok.TokenHash), []byte(hash)) != 1 {
		return store.Token{}, ErrUnknown
	}
	// 异步更新 last_used_at（不阻塞请求）
	go func() {
		_ = a.store.TouchTokenLastUsed(context.Background(), tok.ID)
	}()
	return tok, nil
}

// Verify 校验 Bearer header，返回 token id（owner）。
// 失败时返回具体 error，调用方映射到 401/403。
func (a *Service) Verify(ctx context.Context, bearerHeader string) (string, error) {
	tok, err := a.VerifyToken(ctx, bearerHeader)
	if err != nil {
		return "", err
	}
	return "user:" + strings.TrimSpace(tok.OwnerUserID), nil
}

// HashToken 计算 token 的 SHA-256 hash。
func HashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

func HashPassword(password string) (string, error) {
	salt, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	iterations := 120000
	sum := pbkdf2SHA256([]byte(password), salt, iterations, 32)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", iterations, base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(sum)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hLen := sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, numBlocks*hLen)
	for block := 1; block <= numBlocks; block++ {
		u := hmacSHA256(password, append(append([]byte{}, salt...), byte(block>>24), byte(block>>16), byte(block>>8), byte(block)))
		t := append([]byte{}, u...)
		for i := 1; i < iterations; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func hmacSHA256(key, data []byte) []byte {
	const blockSize = 64
	if len(key) > blockSize {
		sum := sha256.Sum256(key)
		key = sum[:]
	}
	k := make([]byte, blockSize)
	copy(k, key)
	oKey := make([]byte, blockSize)
	iKey := make([]byte, blockSize)
	for i := 0; i < blockSize; i++ {
		oKey[i] = k[i] ^ 0x5c
		iKey[i] = k[i] ^ 0x36
	}
	inner := sha256.New()
	inner.Write(iKey)
	inner.Write(data)
	innerSum := inner.Sum(nil)
	outer := sha256.New()
	outer.Write(oKey)
	outer.Write(innerSum)
	return outer.Sum(nil)
}

func randomBase64(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// List 列出所有 token。includeRevoked=false 时过滤掉已吊销。
func (a *Service) List(ctx context.Context, includeRevoked bool) ([]store.Token, error) {
	toks, err := a.store.ListTokens(ctx)
	if err != nil {
		return nil, err
	}
	if includeRevoked {
		return toks, nil
	}
	out := toks[:0]
	for _, t := range toks {
		if !t.IsRevoked {
			out = append(out, t)
		}
	}
	return out, nil
}

// Revoke 吊销 token。
func (a *Service) Revoke(ctx context.Context, id string) error {
	return a.store.RevokeToken(ctx, id)
}

func (a *Service) GetToken(ctx context.Context, id string) (store.Token, error) {
	return a.store.GetTokenByID(ctx, id)
}

// newUUID 生成 v4 UUID。
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
