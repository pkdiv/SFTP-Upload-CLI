package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/easysftp/sftp-upload/internal/config"
)

type Client struct {
	conn *ssh.Client
}

func KnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

func hostKeyCallback() (ssh.HostKeyCallback, error) {
	path := KnownHostsPath()
	return HostKeyCallbackFromFile(path)
}

func HostKeyCallbackFromFile(path string) (ssh.HostKeyCallback, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return fmt.Errorf(
				"host key verification failed: no known_hosts file at %s; fingerprint %s; add the host key to known_hosts before connecting",
				path, ssh.FingerprintSHA256(key),
			)
		}, nil
	}
	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				return fmt.Errorf(
					"host key verification failed: unknown host key for %s; fingerprint %s; add the host key to known_hosts before connecting",
					hostname, ssh.FingerprintSHA256(key),
				)
			}
			return fmt.Errorf("host key verification failed for %s: %w", hostname, err)
		}
		return nil
	}, nil
}

func loadPrivateKey(path string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}
	var passphraseErr *ssh.PassphraseMissingError
	if errors.As(err, &passphraseErr) {
		return nil, fmt.Errorf("private key %s requires a passphrase, which is not supported in this build", path)
	}
	return nil, fmt.Errorf("parse private key %s: %w", path, err)
}

func Connect(p config.Profile) (*Client, error) {
	return ConnectWithKnownHosts(p, KnownHostsPath())
}

func ConnectWithKnownHosts(p config.Profile, knownHostsPath string) (*Client, error) {
	p.Normalize()
	if p.Port == 0 {
		p.Port = 22
	}

	signer, err := loadPrivateKey(p.KeyFile)
	if err != nil {
		return nil, err
	}

	hkcb, err := HostKeyCallbackFromFile(knownHostsPath)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))
	clientCfg := &ssh.ClientConfig{
		User:            p.Username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hkcb,
		Timeout:         15 * time.Second,
	}

	conn, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}

	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) NewSFTPClient() (*SFTPClient, error) {
	client, err := sftpNewClient(c.conn)
	if err != nil {
		return nil, fmt.Errorf("create SFTP client: %w", err)
	}
	return &SFTPClient{client: client}, nil
}
