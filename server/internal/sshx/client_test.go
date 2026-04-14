package sshx

import (
	"context"
	"io"
	"testing"
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
