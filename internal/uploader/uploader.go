package uploader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/easysftp/sftp-upload/internal/config"
	"github.com/easysftp/sftp-upload/internal/output"
	"github.com/easysftp/sftp-upload/internal/pathutil"
	"github.com/easysftp/sftp-upload/internal/sshclient"
)

type Job struct {
	Index      int
	Total      int
	LocalPath  string
	RemotePath string
	RelPath    string
}

type Result struct {
	Job     Job
	Success bool
	Skipped bool
	Err     error
}

type Options struct {
	Concurrency    int
	CreateRemote   bool
	Confirm        func(job Job) (bool, error)
	OverwriteCheck func(remotePath string) (bool, error)
}

type Uploader struct {
	profile       config.Profile
	out           *output.Writer
	connectFn     func(config.Profile) (*sshclient.Client, error)
}

func New(profile config.Profile, out *output.Writer) *Uploader {
	return &Uploader{
		profile:   profile,
		out:       out,
		connectFn: sshclient.Connect,
	}
}

func NewWithConnect(profile config.Profile, out *output.Writer, connectFn func(config.Profile) (*sshclient.Client, error)) *Uploader {
	return &Uploader{
		profile:   profile,
		out:       out,
		connectFn: connectFn,
	}
}

func (u *Uploader) Resolve(files []string) ([]Job, error) {
	jobs := make([]Job, 0, len(files))
	for i, rel := range files {
		local, err := pathutil.ValidateLocalFile(u.profile.LocalBase, rel)
		if err != nil {
			return nil, fmt.Errorf("file %d (%s): %w", i+1, rel, err)
		}
		remote, err := pathutil.ResolveRemote(u.profile.RemoteBase, rel)
		if err != nil {
			return nil, fmt.Errorf("file %d (%s): %w", i+1, rel, err)
		}
		jobs = append(jobs, Job{
			Index:      i + 1,
			Total:      len(files),
			LocalPath:  local,
			RemotePath: remote,
			RelPath:    rel,
		})
	}
	return jobs, nil
}

func (u *Uploader) Upload(jobs []Job, opts Options) ([]Result, error) {
	results := make([]Result, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)

	for i, job := range jobs {
		approved, err := opts.Confirm(job)
		if err != nil {
			results[i] = Result{Job: job, Skipped: true, Err: err}
			continue
		}
		if !approved {
			results[i] = Result{Job: job, Skipped: true}
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, j Job) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = u.uploadOne(j, opts)
		}(i, job)
	}

	wg.Wait()
	return results, nil
}

func (u *Uploader) uploadOne(job Job, opts Options) Result {
	conn, err := u.connectFn(u.profile)
	if err != nil {
		return Result{Job: job, Err: err}
	}
	defer conn.Close()

	sftpClient, err := conn.NewSFTPClient()
	if err != nil {
		return Result{Job: job, Err: err}
	}
	defer sftpClient.Close()

	exists, err := sftpClient.Exists(job.RemotePath)
	if err != nil {
		return Result{Job: job, Err: fmt.Errorf("check remote file: %w", err)}
	}
	if exists {
		if opts.OverwriteCheck != nil {
			ok, err := opts.OverwriteCheck(job.RemotePath)
			if err != nil {
				return Result{Job: job, Err: err}
			}
			if !ok {
				return Result{Job: job, Skipped: true}
			}
		}
	} else if opts.CreateRemote {
		dir := filepath.ToSlash(filepath.Dir(job.RemotePath))
		if err := sftpClient.MkdirAll(dir); err != nil {
			return Result{Job: job, Err: fmt.Errorf("create remote directory %s: %w", dir, err)}
		}
	}

	localFile, err := os.Open(job.LocalPath)
	if err != nil {
		return Result{Job: job, Err: fmt.Errorf("open local file: %w", err)}
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(job.RemotePath)
	if err != nil {
		return Result{Job: job, Err: fmt.Errorf("create remote file: %w", err)}
	}

	u.out.Printf("[%d/%d] Uploading %s\n", job.Index, job.Total, job.RelPath)

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		remoteFile.Close()
		return Result{Job: job, Err: fmt.Errorf("copy file: %w", err)}
	}
	if err := remoteFile.Close(); err != nil {
		return Result{Job: job, Err: fmt.Errorf("close remote file: %w", err)}
	}

	u.out.Printf("[%d/%d] ✓ Uploaded %s\n", job.Index, job.Total, job.RelPath)
	return Result{Job: job, Success: true}
}

func (u *Uploader) Summarize(results []Result) (uploaded, skipped, failed int) {
	for _, r := range results {
		switch {
		case r.Success:
			uploaded++
		case r.Skipped:
			skipped++
		default:
			failed++
		}
	}
	return
}
