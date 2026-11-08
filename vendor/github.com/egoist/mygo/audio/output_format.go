package audio

import (
	"fmt"
	"runtime"
)

// Local extension: Windows can open each track at its decoded sample rate.
// requestedFormat and mixRate are protected by mu; other platforms retain 48 kHz.
type outputFormat struct {
	rate      int
	exclusive bool
}

var requestedFormat = outputFormat{rate: sampleRate}
var mixRate = sampleRate
var outputStatus = "尚未打开音频设备"
var outputMixStatus = "尚未读取"

// OutputMixStatus is the engine's shared mix format at the last successful open.
// It is not the hardware/DAC format, and exclusive playback bypasses this mixer.
func OutputMixStatus() string {
	errMu.Lock()
	defer errMu.Unlock()
	return outputMixStatus
}

// ConfigureOutput selects the next Windows stream's sample rate and mode.
// Reopening is done on the output thread, never on the caller's UI thread.
func ConfigureOutput(rate int, exclusive bool) {
	if runtime.GOOS != "windows" {
		return
	}
	if rate <= 0 {
		rate = sampleRate
	}
	mu.Lock()
	defer mu.Unlock()
	f := outputFormat{rate, exclusive}
	if requestedFormat != f {
		requestedFormat = f
		wakeOutput()
	}
}

func outputSettings() outputFormat {
	mu.Lock()
	defer mu.Unlock()
	return requestedFormat
}

// OutputStatus describes the last successfully opened stream, not its source.
func OutputStatus() string {
	errMu.Lock()
	defer errMu.Unlock()
	if outErr != nil {
		return "输出不可用：" + outErr.Error()
	}
	return outputStatus
}

func setOutputStatus(rate, bits int, exclusive bool, fallback error) {
	mode := "共享，Windows 可能按设备格式转换"
	if exclusive {
		mode = "独占"
	}
	text := fmt.Sprintf("%g kHz / %d 位 · %s", float64(rate)/1000, bits, mode)
	if fallback != nil {
		text += "；独占未启用：" + fallback.Error()
	}
	errMu.Lock()
	outputStatus = text
	errMu.Unlock()
}
