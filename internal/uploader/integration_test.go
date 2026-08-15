package uploader

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/easysftp/sftp-upload/internal/config"
	"github.com/easysftp/sftp-upload/internal/output"
	"github.com/easysftp/sftp-upload/internal/sshclient"
)

type testServer struct {
	listener net.Listener
	hostKey  ssh.Signer
	rootDir  string
	done     chan struct{}
}

func startTestServer(t *testing.T) (*testServer, string) {
	t.Helper()

	rootDir := t.TempDir()
	hostKey, err := generateHostKey()
	if err != nil {
		t.Fatal(err)
	}

	serverCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	serverCfg.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srv := &testServer{
		listener: listener,
		hostKey:  hostKey,
		rootDir:  rootDir,
		done:     make(chan struct{}),
	}

	go srv.serve(serverCfg)
	t.Cleanup(func() {
		srv.Close()
	})

	return srv, rootDir
}

func generateHostKey() (ssh.Signer, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(privateKey)
}

func (s *testServer) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}
	s.listener.Close()
}

func (s *testServer) serve(cfg *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				return
			}
		}
		go s.handleConn(conn, cfg)
	}
}

func (s *testServer) handleConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	defer nConn.Close()

	_, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}

		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := false
				if req.Type == "subsystem" && string(req.Payload[4:]) == "sftp" {
					ok = true
				}
				req.Reply(ok, nil)
			}
		}(requests)

		server, err := sftp.NewServer(channel, sftp.WindowsRootEnumeratesDrives())
		if err != nil {
			channel.Close()
			continue
		}
		server.Serve()
		server.Close()
	}
}

func (s *testServer) addr() string {
	return s.listener.Addr().String()
}

func writeKnownHosts(t *testing.T, srv *testServer) string {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.addr())
	if err != nil {
		t.Fatal(err)
	}

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")

	f, err := os.OpenFile(knownHostsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Write a known_hosts line: [host]:port keytype base64key
	marshaled := ssh.MarshalAuthorizedKey(srv.hostKey.PublicKey())
	line := fmt.Sprintf("[%s]:%s %s", host, port, string(marshaled))
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}

	return knownHostsPath
}

func writeClientKey(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	privPath := filepath.Join(dir, "client_key")

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	privBytes := marshalED25519PEM(priv)
	if err := os.WriteFile(privPath, privBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	return privPath, dir
}

func TestUploaderIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	srv, rootDir := startTestServer(t)
	host, port, _ := net.SplitHostPort(srv.addr())

	knownHostsPath := writeKnownHosts(t, srv)
	clientKeyPath, _ := writeClientKey(t)

	localBase := t.TempDir()
	// Create local files
	file1 := filepath.Join(localBase, "src", "main.go")
	file2 := filepath.Join(localBase, "config", "app.yaml")
	if err := os.MkdirAll(filepath.Dir(file1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(file2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file1, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("app: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile := config.Profile{
		Host:       host,
		Port:       mustAtoi(port),
		Username:   "testuser",
		KeyFile:    clientKeyPath,
		LocalBase:  localBase,
		RemoteBase: toRemotePath(rootDir),
	}
	profile.Normalize()

	connectFn := func(p config.Profile) (*sshclient.Client, error) {
		return sshclient.ConnectWithKnownHosts(p, knownHostsPath)
	}

	var out bytes.Buffer
	writer := output.New(&out)
	up := NewWithConnect(profile, writer, connectFn)

	jobs, err := up.Resolve([]string{"src/main.go", "config/app.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	results, err := up.Upload(jobs, Options{
		Concurrency:  2,
		CreateRemote: true,
		Confirm: func(job Job) (bool, error) {
			return true, nil
		},
		OverwriteCheck: func(remotePath string) (bool, error) {
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	uploaded, skipped, failed := up.Summarize(results)
	if failed != 0 || uploaded != 2 {
		t.Fatalf("expected 2 uploaded 0 failed, got uploaded=%d skipped=%d failed=%d", uploaded, skipped, failed)
		for _, r := range results {
			if r.Err != nil {
				t.Logf("job %s failed: %v", r.Job.RelPath, r.Err)
			}
		}
	}

	// Verify remote files were written correctly
	verifyRemoteFile(t, rootDir, toRemotePath(rootDir)+"/src/main.go", "package main\n")
	verifyRemoteFile(t, rootDir, toRemotePath(rootDir)+"/config/app.yaml", "app: test\n")

	if !strings.Contains(out.String(), "✓ Uploaded") {
		t.Fatalf("expected success output, got %q", out.String())
	}
}

func verifyRemoteFile(t *testing.T, rootDir, remotePath, want string) {
	t.Helper()
	// remotePath is a Unix-style path like "/C:/Users/.../file.txt".
	// On Windows the SFTP server maps "/C:/..." to "C:\...".
	// Just strip the leading "/" and convert slashes.
	path := filepath.FromSlash(strings.TrimPrefix(remotePath, "/"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("remote file %s not readable: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("remote file %s = %q, want %q", path, data, want)
	}
}

func toRemotePath(localPath string) string {
	// On Windows, the SFTP server treats "/C:/..." as "C:\..."
	// So we convert a Windows drive path to an SFTP-friendly absolute path.
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return localPath
	}
	vol := filepath.VolumeName(abs)
	rest := abs[len(vol):]
	rest = strings.ReplaceAll(rest, "\\", "/")
	if vol == "" {
		return "/" + strings.TrimPrefix(rest, "/")
	}
	return "/" + strings.TrimSuffix(vol, ":") + ":" + rest
}

func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
