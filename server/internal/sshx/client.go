package sshx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Target struct {
	Host                      string
	Port                      int
	Username                  string
	Password                  string
	PrivateKeyPEM             string
	TrustedHostKeyFingerprint string
	AllowUnknownHostKey       bool
	Timeout                   time.Duration
}

type Runner interface {
	Run(ctx context.Context, target Target, command string) (string, string, int, error)
}

type HostKeyFingerprintReader interface {
	GetHostKeyFingerprint(ctx context.Context, target Target) (string, error)
}

type HostKeyMismatchError struct {
	Expected string
	Actual   string
}

func (e HostKeyMismatchError) Error() string {
	return fmt.Sprintf("SSH 主机指纹不匹配，期望 %s，实际 %s", e.Expected, e.Actual)
}

type HostKeyTrustRequiredError struct {
	Actual string
}

func (e HostKeyTrustRequiredError) Error() string {
	if strings.TrimSpace(e.Actual) == "" {
		return "SSH 主机指纹尚未信任"
	}
	return fmt.Sprintf("SSH 主机指纹尚未信任: %s", e.Actual)
}

type sessionClient interface {
	NewSession() (sessionRunner, error)
	Close() error
}

type sessionRunner interface {
	SetStdout(io.Writer)
	SetStderr(io.Writer)
	Run(command string) error
	Close() error
}

type Client struct {
	open func(ctx context.Context, target Target) (sessionClient, error)
}

func NewClient() *Client {
	return &Client{open: openSSHClient}
}

func (c *Client) Run(ctx context.Context, target Target, command string) (string, string, int, error) {
	open := c.open
	if open == nil {
		open = openSSHClient
	}

	client, err := open(ctx, target)
	if err != nil {
		return "", "", -1, err
	}
	defer func() {
		_ = client.Close()
	}()

	session, err := client.NewSession()
	if err != nil {
		return "", "", -1, err
	}
	defer func() {
		_ = session.Close()
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.SetStdout(&stdout)
	session.SetStderr(&stderr)

	if err := ctx.Err(); err != nil {
		return "", "", -1, err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Run(command)
	}()

	select {
	case err = <-errCh:
	case <-ctx.Done():
		_ = session.Close()
		_ = client.Close()
		select {
		case <-errCh:
		case <-time.After(100 * time.Millisecond):
		}
		return stdout.String(), stderr.String(), -1, ctx.Err()
	}
	if err != nil {
		if exitCode, ok := exitCodeFromError(err); ok {
			return stdout.String(), stderr.String(), exitCode, nil
		}
		return stdout.String(), stderr.String(), -1, err
	}

	return stdout.String(), stderr.String(), 0, nil
}

func (c *Client) GetHostKeyFingerprint(ctx context.Context, target Target) (string, error) {
	target.AllowUnknownHostKey = true
	capture := &hostKeyCapture{}
	config, err := buildSSHConfig(target, capture)
	if err != nil {
		return "", err
	}

	conn, err := (&net.Dialer{Timeout: target.timeoutOrDefault()}).DialContext(ctx, "tcp", target.Address())
	if err != nil {
		return "", err
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, target.Address(), config)
	if err != nil {
		_ = conn.Close()
		if capture.actual != "" {
			var mismatch HostKeyMismatchError
			if errors.As(err, &mismatch) {
				return mismatch.Actual, err
			}
			return capture.actual, nil
		}
		return "", err
	}

	client := ssh.NewClient(clientConn, chans, reqs)
	_ = client.Close()
	return capture.actual, nil
}

type sshClientWrapper struct {
	client *ssh.Client
}

func (c *sshClientWrapper) NewSession() (sessionRunner, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	return &sshSessionWrapper{session: session}, nil
}

func (c *sshClientWrapper) Close() error {
	return c.client.Close()
}

type sshSessionWrapper struct {
	session *ssh.Session
}

func (s *sshSessionWrapper) SetStdout(w io.Writer) {
	s.session.Stdout = w
}

func (s *sshSessionWrapper) SetStderr(w io.Writer) {
	s.session.Stderr = w
}

func (s *sshSessionWrapper) Run(command string) error {
	return s.session.Run(command)
}

func (s *sshSessionWrapper) Close() error {
	return s.session.Close()
}

type hostKeyCapture struct {
	actual string
}

func (c *hostKeyCapture) callback(target Target) ssh.HostKeyCallback {
	expected := strings.TrimSpace(target.TrustedHostKeyFingerprint)
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		c.actual = ssh.FingerprintSHA256(key)
		if expected == "" {
			if target.AllowUnknownHostKey {
				return nil
			}
			return HostKeyTrustRequiredError{Actual: c.actual}
		}
		if !strings.EqualFold(expected, c.actual) {
			return HostKeyMismatchError{Expected: expected, Actual: c.actual}
		}
		return nil
	}
}

func openSSHClient(ctx context.Context, target Target) (sessionClient, error) {
	capture := &hostKeyCapture{}
	config, err := buildSSHConfig(target, capture)
	if err != nil {
		return nil, err
	}

	conn, err := (&net.Dialer{Timeout: target.timeoutOrDefault()}).DialContext(ctx, "tcp", target.Address())
	if err != nil {
		return nil, err
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, target.Address(), config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &sshClientWrapper{client: ssh.NewClient(clientConn, chans, reqs)}, nil
}

func buildSSHConfig(target Target, capture *hostKeyCapture) (*ssh.ClientConfig, error) {
	if target.Host == "" {
		return nil, errors.New("SSH 目标主机不能为空")
	}
	if target.Username == "" {
		return nil, errors.New("SSH 登录用户名不能为空")
	}
	if capture == nil {
		capture = &hostKeyCapture{}
	}

	authMethods, err := buildAuthMethods(target)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User:            target.Username,
		Auth:            authMethods,
		HostKeyCallback: capture.callback(target),
		Timeout:         target.timeoutOrDefault(),
	}, nil
}

func buildAuthMethods(target Target) ([]ssh.AuthMethod, error) {
	authMethods := make([]ssh.AuthMethod, 0, 2)
	if target.Password != "" {
		authMethods = append(authMethods, ssh.Password(target.Password))
	}
	if target.PrivateKeyPEM != "" {
		signer, err := ssh.ParsePrivateKey([]byte(target.PrivateKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("SSH 私钥格式无效: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if len(authMethods) == 0 {
		return nil, errors.New("SSH 凭据不能为空")
	}
	return authMethods, nil
}

func exitCodeFromError(err error) (int, bool) {
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Waitmsg.ExitStatus(), true
	}

	statusErr, ok := err.(interface{ ExitStatus() int })
	if ok {
		return statusErr.ExitStatus(), true
	}
	return 0, false
}

func (t Target) Address() string {
	port := t.Port
	if port == 0 {
		port = 22
	}
	return fmt.Sprintf("%s:%d", t.Host, port)
}

func (t Target) timeoutOrDefault() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return 10 * time.Second
}
