package sshx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

type Target struct {
	Host          string
	Port          int
	Username      string
	Password      string
	PrivateKeyPEM string
	Timeout       time.Duration
}

type Runner interface {
	Run(ctx context.Context, target Target, command string) (string, string, int, error)
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

	err = session.Run(command)
	if err != nil {
		if exitCode, ok := exitCodeFromError(err); ok {
			return stdout.String(), stderr.String(), exitCode, nil
		}
		return stdout.String(), stderr.String(), -1, err
	}

	return stdout.String(), stderr.String(), 0, nil
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

func openSSHClient(ctx context.Context, target Target) (sessionClient, error) {
	if target.Host == "" {
		return nil, errors.New("SSH 目标主机不能为空")
	}
	if target.Username == "" {
		return nil, errors.New("SSH 登录用户名不能为空")
	}

	config := &ssh.ClientConfig{
		User:            target.Username,
		Auth:            buildAuthMethods(target),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         target.timeoutOrDefault(),
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

func buildAuthMethods(target Target) []ssh.AuthMethod {
	authMethods := make([]ssh.AuthMethod, 0, 2)
	if target.Password != "" {
		authMethods = append(authMethods, ssh.Password(target.Password))
	}
	if target.PrivateKeyPEM != "" {
		if signer, err := ssh.ParsePrivateKey([]byte(target.PrivateKeyPEM)); err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	return authMethods
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
