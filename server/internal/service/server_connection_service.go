package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"hostdeck/server/internal/credential"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

var ErrConnectionServerNotFound = errors.New("服务器不存在")
var ErrServerDisabled = errors.New("服务器已禁用")
var ErrServerPasswordNotConfigured = errors.New("服务器未配置 SSH 密码")

type ConnectionServerStore interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
	UpdateTrustedHostKeyFingerprint(ctx context.Context, id int64, fingerprint string) error
}

type ConnectionCredentialStore interface {
	GetByServerID(ctx context.Context, serverID int64) (storage.ServerCredential, error)
}

type ServerConnectionService struct {
	servers     ConnectionServerStore
	credentials ConnectionCredentialStore
	masterKey   string
}

func NewServerConnectionService(
	servers ConnectionServerStore,
	credentials ConnectionCredentialStore,
	masterKey string,
) *ServerConnectionService {
	return &ServerConnectionService{
		servers:     servers,
		credentials: credentials,
		masterKey:   strings.TrimSpace(masterKey),
	}
}

func (s *ServerConnectionService) ResolveServer(ctx context.Context, serverID int64) (domain.Server, error) {
	servers, err := s.servers.List(ctx, storage.ServerFilter{ID: serverID})
	if err != nil {
		return domain.Server{}, err
	}
	if len(servers) == 0 {
		return domain.Server{}, ErrConnectionServerNotFound
	}

	server := servers[0]
	if !server.Enabled {
		return domain.Server{}, ErrServerDisabled
	}

	credentialItem, err := s.credentials.GetByServerID(ctx, serverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Server{}, ErrServerPasswordNotConfigured
		}
		return domain.Server{}, err
	}
	if strings.TrimSpace(credentialItem.PasswordCiphertext) == "" {
		return domain.Server{}, ErrServerPasswordNotConfigured
	}

	cipher, err := credential.NewCipher(s.masterKey)
	if err != nil {
		return domain.Server{}, err
	}
	password, err := cipher.Decrypt(credentialItem.PasswordCiphertext)
	if err != nil {
		return domain.Server{}, fmt.Errorf("解密服务器密码失败: %w", err)
	}

	server.Password = password
	server.PasswordConfigured = true
	return server, nil
}

func (s *ServerConnectionService) TrustHostKeyFingerprint(ctx context.Context, serverID int64, fingerprint string) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return errors.New("SSH 主机指纹不能为空")
	}
	if err := s.servers.UpdateTrustedHostKeyFingerprint(ctx, serverID, fingerprint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConnectionServerNotFound
		}
		return err
	}
	return nil
}
