package mirror

import (
	"context"
	"io"

	"github.com/getlatentic/haas/internal/helper"
)

// Remote is the agent's folder in the cloud, as the sync needs it.
type Remote interface {
	Files(ctx context.Context) (files []helper.RemoteFile, cut bool, err error)
	Hashes(ctx context.Context, paths []string) (map[string]string, error)
	Download(ctx context.Context, path string, w io.Writer) error
	Upload(ctx context.Context, path string, body io.Reader, saw string, executable bool) (string, error)
	Remove(ctx context.Context, path, saw string) error
}

// Cloud is one agent's folder reached with a person's token.
type Cloud struct {
	Client   helper.Client
	Instance string
}

func (c Cloud) Files(ctx context.Context) ([]helper.RemoteFile, bool, error) {
	return c.Client.Files(ctx, c.Instance)
}

func (c Cloud) Hashes(ctx context.Context, paths []string) (map[string]string, error) {
	return c.Client.Hashes(ctx, c.Instance, paths)
}

func (c Cloud) Download(ctx context.Context, path string, w io.Writer) error {
	return c.Client.Download(ctx, c.Instance, path, w)
}

func (c Cloud) Upload(ctx context.Context, path string, body io.Reader, saw string, executable bool) (string, error) {
	return c.Client.Upload(ctx, c.Instance, path, body, saw, executable)
}

func (c Cloud) Remove(ctx context.Context, path, saw string) error {
	return c.Client.Remove(ctx, c.Instance, path, saw)
}
