package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const infoContext = "hostdeck/server-credentials"

type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(masterKey string) (*Cipher, error) {
	trimmed := strings.TrimSpace(masterKey)
	if trimmed == "" || trimmed == "change-me" {
		return nil, errors.New("master_key 未配置，无法安全保存服务器密码")
	}
	if len(trimmed) < 16 {
		return nil, errors.New("master_key 长度不足，至少需要 16 个字符")
	}

	reader := hkdf.New(sha256.New, []byte(trimmed), nil, []byte(infoContext))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("派生凭据密钥失败: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES 加密器失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 加密器失败: %w", err)
	}

	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plainText string) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("凭据加密器尚未初始化")
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机 nonce 失败: %w", err)
	}

	cipherText := c.aead.Seal(nil, nonce, []byte(plainText), nil)
	encoded := append(nonce, cipherText...)
	return base64.StdEncoding.EncodeToString(encoded), nil
}

func (c *Cipher) Decrypt(encoded string) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("凭据加密器尚未初始化")
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("解析密文失败: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("凭据密文格式不正确")
	}

	nonce := raw[:nonceSize]
	cipherText := raw[nonceSize:]
	plainText, err := c.aead.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", fmt.Errorf("解密凭据密文失败: %w", err)
	}
	return string(plainText), nil
}
