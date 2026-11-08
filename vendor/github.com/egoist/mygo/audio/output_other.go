//go:build !darwin && !(linux && (amd64 || arm64)) && !(windows && (amd64 || arm64)) && !js

package audio

import "time"

func openDevice(ahead time.Duration, format outputFormat) (device, int, error) {
	return nil, 0, errUnsupported
}
