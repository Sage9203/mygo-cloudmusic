//go:build windows && (amd64 || arm64)

package audio

import (
	"fmt"
	"unsafe"
)

// readMixFormat observes the engine without initializing playback or changing settings.
func readMixFormat(client uintptr) string {
	var mix *waveFormatExtensible
	if hr := comCall(client, 8, uintptr(unsafe.Pointer(&mix))); failed(hr) {
		return "读取失败：" + hresult("GetMixFormat", hr).Error()
	}
	if mix == nil {
		return "未返回混音格式"
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(mix)))
	return describeMixFormat(mix)
}

func describeMixFormat(f *waveFormatExtensible) string {
	bits, kind := f.bitsPerSample, "PCM"
	if f.formatTag == 3 {
		kind = "浮点"
	}
	if f.formatTag == waveFormatExtensibleTag && f.size >= 22 {
		if f.validBits > 0 {
			bits = f.validBits
		}
		if f.subFormat == ksDataFormatSubtypeFloat {
			kind = "浮点"
		}
	}
	return fmt.Sprintf("%g kHz / %d 位%s / %d 声道", float64(f.samplesPerSec)/1000, bits, kind, f.channels)
}
