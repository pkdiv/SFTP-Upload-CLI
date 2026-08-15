package uploader

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkdiv/uplift/internal/config"
	"github.com/pkdiv/uplift/internal/output"
	"github.com/pkdiv/uplift/internal/sshclient"
)

func TestConcurrencyLimitEnforced(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	srv, rootDir := startTestServer(t)
	host, port, _ := net.SplitHostPort(srv.addr())
	knownHostsPath := writeKnownHosts(t, srv)
	clientKeyPath, _ := writeClientKey(t)

	localBase := t.TempDir()
	var files []string
	for i := 0; i < 6; i++ {
		f := filepath.Join(localBase, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("content %d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, fmt.Sprintf("file%d.txt", i))
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

	jobs, err := up.Resolve(files)
	if err != nil {
		t.Fatal(err)
	}

	concurrency := 2

	results, err := up.Upload(jobs, Options{
		Concurrency:  concurrency,
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
	if failed != 0 || uploaded != 6 {
		t.Fatalf("expected 6 uploaded, got uploaded=%d skipped=%d failed=%d", uploaded, skipped, failed)
	}
}

func TestOverwriteDeclinedSkipsUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	srv, rootDir := startTestServer(t)
	host, port, _ := net.SplitHostPort(srv.addr())
	knownHostsPath := writeKnownHosts(t, srv)
	clientKeyPath, _ := writeClientKey(t)

	localBase := t.TempDir()
	localFile := filepath.Join(localBase, "existing.txt")
	if err := os.WriteFile(localFile, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-create remote file with old content
	remoteRoot := toRemotePath(rootDir)
	remoteFileLocal := remoteToLocalPath(remoteRoot + "/existing.txt")
	if err := os.MkdirAll(filepath.Dir(remoteFileLocal), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remoteFileLocal, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile := config.Profile{
		Host:       host,
		Port:       mustAtoi(port),
		Username:   "testuser",
		KeyFile:    clientKeyPath,
		LocalBase:  localBase,
		RemoteBase: remoteRoot,
	}
	profile.Normalize()

	connectFn := func(p config.Profile) (*sshclient.Client, error) {
		return sshclient.ConnectWithKnownHosts(p, knownHostsPath)
	}

	var out bytes.Buffer
	writer := output.New(&out)
	up := NewWithConnect(profile, writer, connectFn)

	jobs, err := up.Resolve([]string{"existing.txt"})
	if err != nil {
		t.Fatal(err)
	}

	results, err := up.Upload(jobs, Options{
		Concurrency:  1,
		CreateRemote: true,
		Confirm: func(job Job) (bool, error) {
			return true, nil
		},
		OverwriteCheck: func(remotePath string) (bool, error) {
			return false, nil // user declines overwrite
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	uploaded, skipped, failed := up.Summarize(results)
	if uploaded != 0 || skipped != 1 || failed != 0 {
		t.Fatalf("expected 0 uploaded 1 skipped 0 failed, got uploaded=%d skipped=%d failed=%d", uploaded, skipped, failed)
	}

	// Verify remote file still has old content
	data, err := os.ReadFile(remoteFileLocal)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old content" {
		t.Fatalf("remote file was overwritten: got %q", data)
	}
}

func TestUploaderResolveRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	profile := config.Profile{LocalBase: dir, RemoteBase: "/var/www/app"}
	up := New(profile, output.New(&bytes.Buffer{}))

	_, err := up.Resolve([]string{"../escape.txt"})
	if err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestUploaderResolveRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	profile := config.Profile{LocalBase: dir, RemoteBase: "/var/www/app"}
	up := New(profile, output.New(&bytes.Buffer{}))

	_, err := up.Resolve([]string{"missing.txt"})
	if err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestGenerateKeyPEMRoundtrip(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pemData := marshalED25519PEM(priv)
	if len(pemData) == 0 {
		t.Fatal("empty PEM")
	}
}


