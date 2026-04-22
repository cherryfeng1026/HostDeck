package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

type fakeClient struct {
	session sessionRunner
}

func (c fakeClient) NewSession() (sessionRunner, error) {
	return c.session, nil
}

func (c fakeClient) Close() error {
	return nil
}

type fakeSession struct {
	stdoutData string
	stderrData string
	runErr     error
	stdout     io.Writer
	stderr     io.Writer
}

func (s *fakeSession) SetStdout(w io.Writer) {
	s.stdout = w
}

func (s *fakeSession) SetStderr(w io.Writer) {
	s.stderr = w
}

func (s *fakeSession) Run(string) error {
	if s.stdout != nil {
		_, _ = io.WriteString(s.stdout, s.stdoutData)
	}
	if s.stderr != nil {
		_, _ = io.WriteString(s.stderr, s.stderrData)
	}
	return s.runErr
}

func (s *fakeSession) Close() error {
	return nil
}

type fakeExitError struct {
	code int
}

func (e fakeExitError) Error() string {
	return "remote command failed"
}

func (e fakeExitError) ExitStatus() int {
	return e.code
}

func TestClientRunCapturesOutput(t *testing.T) {
	client := Client{
		open: func(context.Context, Target) (sessionClient, error) {
			return fakeClient{
				session: &fakeSession{stdoutData: "linux\n", stderrData: ""},
			}, nil
		},
	}

	stdout, stderr, exitCode, err := client.Run(context.Background(), Target{Host: "10.0.0.21", Username: "root"}, "uname -s")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if stdout != "linux\n" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestClientRunReturnsRemoteExitCode(t *testing.T) {
	client := Client{
		open: func(context.Context, Target) (sessionClient, error) {
			return fakeClient{
				session: &fakeSession{
					stdoutData: "",
					stderrData: "command not found\n",
					runErr:     fakeExitError{code: 127},
				},
			}, nil
		},
	}

	stdout, stderr, exitCode, err := client.Run(context.Background(), Target{Host: "10.0.0.21", Username: "root"}, "bad-command")
	if err != nil {
		t.Fatalf("expected nil error for remote exit status, got %v", err)
	}
	if stdout != "" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "command not found\n" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if exitCode != 127 {
		t.Fatalf("expected exit code 127, got %d", exitCode)
	}
}

func TestHostKeyCaptureAcceptsMatchingFingerprint(t *testing.T) {
	publicKey := newTestPublicKey(t)
	fingerprint := ssh.FingerprintSHA256(publicKey)
	capture := &hostKeyCapture{}
	callback := capture.callback(fingerprint)
	if err := callback("", nil, publicKey); err != nil {
		t.Fatalf("expected matching fingerprint to pass, got %v", err)
	}
	if capture.actual != fingerprint {
		t.Fatalf("unexpected captured fingerprint: %q", capture.actual)
	}
}

func TestHostKeyCaptureRejectsMismatchedFingerprint(t *testing.T) {
	publicKey := newTestPublicKey(t)
	actual := ssh.FingerprintSHA256(publicKey)
	capture := &hostKeyCapture{}
	err := capture.callback("SHA256:trusted")("", nil, publicKey)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	mismatch, ok := err.(HostKeyMismatchError)
	if !ok {
		t.Fatalf("expected HostKeyMismatchError, got %T", err)
	}
	if mismatch.Expected != "SHA256:trusted" || mismatch.Actual != actual {
		t.Fatalf("unexpected mismatch error: %+v", mismatch)
	}
	if !strings.Contains(err.Error(), "SSH 主机指纹不匹配") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func newTestPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer.PublicKey()
}

func TestTargetAddressDefaultsToPort22(t *testing.T) {
	target := Target{Host: "10.0.0.21"}
	if target.Address() != "10.0.0.21:22" {
		t.Fatalf("unexpected address: %q", target.Address())
	}
}
