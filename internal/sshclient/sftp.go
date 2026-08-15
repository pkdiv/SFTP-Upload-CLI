package sshclient

import (
	"io"
	"os"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPClient struct {
	client *sftp.Client
}

func sftpNewClient(conn *ssh.Client) (*sftp.Client, error) {
	return sftp.NewClient(conn)
}

func (c *SFTPClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *SFTPClient) Stat(path string) (os.FileInfo, error) {
	return c.client.Stat(path)
}

func (c *SFTPClient) MkdirAll(path string) error {
	return c.client.MkdirAll(path)
}

func (c *SFTPClient) OpenFile(path string) (io.ReadWriteCloser, error) {
	return c.client.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
}

func (c *SFTPClient) Remove(path string) error {
	return c.client.Remove(path)
}

func (c *SFTPClient) Create(path string) (io.WriteCloser, error) {
	return c.client.Create(path)
}

func (c *SFTPClient) Exists(path string) (bool, error) {
	_, err := c.client.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func IsNotExist(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	return strings.Contains(err.Error(), "does not exist")
}
