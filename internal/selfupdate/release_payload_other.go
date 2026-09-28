//go:build !windows

package selfupdate

import (
	"context"
	"errors"
)

type windowsReleasePayload struct {
	CorePath    string
	BundlePath  string
	DesktopPath string
}

func extractWindowsReleasePayload(context.Context, []byte, string, string, string) (windowsReleasePayload, error) {
	return windowsReleasePayload{}, errors.New("Windows Release payload extraction is unavailable on this platform")
}
