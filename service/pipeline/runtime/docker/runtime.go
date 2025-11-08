package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	containertypes "github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type Runtime struct {
	client *client.Client
	pulled sync.Map
}

func NewRuntime() (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Runtime{client: cli}, nil
}

// Run creates, attaches, waits and removes a container based on the provided configuration.
func (r *Runtime) Run(ctx context.Context, cfg ContainerConfig, logFn func(string) error) (int, error) {
	if err := r.ensureImage(ctx, cfg.Image, logFn); err != nil {
		return -1, err
	}

	containerCfg, hostCfg := toDockerConfigs(cfg)
	resp, err := r.client.ContainerCreate(ctx, containerCfg, hostCfg, &network.NetworkingConfig{}, nil, cfg.Name)
	if err != nil {
		return -1, err
	}
	id := resp.ID
	defer r.removeContainer(context.Background(), id)

	if err := r.client.ContainerStart(ctx, id, containertypes.StartOptions{}); err != nil {
		return -1, err
	}

	attach, err := r.client.ContainerAttach(ctx, id, containertypes.AttachOptions{Stream: true, Stdout: true, Stderr: true})
	if err != nil {
		return -1, err
	}
	defer attach.Close()

	writer := newLogWriter(logFn)
	logDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(writer, writer, attach.Reader)
		writer.Flush()
		logDone <- err
	}()

	statusCh, errCh := r.client.ContainerWait(ctx, id, containertypes.WaitConditionNotRunning)

	exitCode := 0
	var runErr error

	select {
	case err := <-errCh:
		if err != nil {
			runErr = err
		}
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
		if status.Error != nil {
			runErr = errors.New(status.Error.Message)
		}
		if status.StatusCode != 0 && runErr == nil {
			runErr = fmt.Errorf("container exited with status %d", status.StatusCode)
		}
	case <-ctx.Done():
		_ = r.client.ContainerStop(context.Background(), id, containertypes.StopOptions{})
		exitCode = -1
		runErr = ctx.Err()
	}

	if err := <-logDone; err != nil && runErr == nil {
		runErr = err
	}

	return exitCode, runErr
}

func (r *Runtime) removeContainer(ctx context.Context, id string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = r.client.ContainerRemove(ctx, id, containertypes.RemoveOptions{Force: true, RemoveVolumes: true})
}

func (r *Runtime) ensureImage(ctx context.Context, image string, logFn func(string) error) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("container image is required")
	}
	if _, ok := r.pulled.Load(image); ok {
		return nil
	}
	if _, _, err := r.client.ImageInspectWithRaw(ctx, image); err == nil {
		r.pulled.Store(image, struct{}{})
		return nil
	} else if err != nil && !client.IsErrNotFound(err) {
		return err
	}

	if logFn != nil {
		_ = logFn(fmt.Sprintf("拉取镜像 %s ...", image))
	}
	reader, err := r.client.ImagePull(ctx, image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("拉取镜像 %s 失败: %w", image, err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	r.pulled.Store(image, struct{}{})
	return nil
}

type ContainerConfig struct {
	Name       string
	Image      string
	Cmd        []string
	Entrypoint []string
	Env        []string
	WorkingDir string
	Volumes    map[string]struct{}
	Binds      []string
	Privileged bool
	Network    string
}

func toDockerConfigs(cfg ContainerConfig) (*containertypes.Config, *containertypes.HostConfig) {
	config := &containertypes.Config{
		Image:      cfg.Image,
		Cmd:        cfg.Cmd,
		Entrypoint: cfg.Entrypoint,
		Env:        cfg.Env,
		WorkingDir: cfg.WorkingDir,
		Volumes:    cfg.Volumes,
	}
	host := &containertypes.HostConfig{
		Binds:       cfg.Binds,
		Privileged:  cfg.Privileged,
		NetworkMode: containertypes.NetworkMode(cfg.Network),
	}
	return config, host
}

type logWriter struct {
	fn  func(string) error
	mu  sync.Mutex
	buf bytes.Buffer
}

func newLogWriter(fn func(string) error) *logWriter {
	return &logWriter{fn: fn}
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	total := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			w.buf.Write(p)
			break
		}
		w.buf.Write(p[:i])
		w.flushLocked()
		p = p[i+1:]
	}
	return total, nil
}

func (w *logWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLocked()
}

func (w *logWriter) flushLocked() {
	if w.buf.Len() == 0 {
		return
	}
	line := w.buf.String()
	w.buf.Reset()
	if w.fn != nil {
		_ = w.fn(line)
	}
}
