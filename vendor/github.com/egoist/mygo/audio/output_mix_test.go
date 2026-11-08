//go:build windows && (amd64 || arm64)

package audio

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Shared float and extensible integer formats must retain their distinct precision and rate.
func TestDescribeMixFormat(t *testing.T) {
	f := waveFormatExtensible{formatTag: 3, channels: 2, samplesPerSec: 48000, bitsPerSample: 32}
	if got := describeMixFormat(&f); !strings.Contains(got, "48 kHz / 32 位浮点") {
		t.Fatal(got)
	}
	f.formatTag, f.size, f.validBits, f.samplesPerSec = waveFormatExtensibleTag, 22, 24, 96000
	if got := describeMixFormat(&f); !strings.Contains(got, "96 kHz / 24 位PCM") {
		t.Fatal(got)
	}
}

// Observe the real shared mixer without Initialize, starting sound or taking exclusive access.
func TestReadOnlyMixFormat(t *testing.T) {
	if os.Getenv("MYGO_MIX_PROBE") != "1" {
		t.Skip("set MYGO_MIX_PROBE for read-only device observation")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded); failed(hr) {
		t.Fatal(hresult("CoInitializeEx", hr))
	}
	defer ole32.NewProc("CoUninitialize").Call()
	var enumerator, dev, client uintptr
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll, uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enumerator))); failed(hr) {
		t.Fatal(hresult("enumerator", hr))
	}
	defer release(enumerator)
	if hr := comCall(enumerator, 4, eRender, eConsole, uintptr(unsafe.Pointer(&dev))); failed(hr) {
		t.Fatal(hresult("default endpoint", hr))
	}
	defer release(dev)
	if hr := comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))); failed(hr) {
		t.Fatal(hresult("activate", hr))
	}
	defer release(client)
	got := readMixFormat(client)
	t.Log("shared engine format:", got)
	if !strings.Contains(got, "kHz") {
		t.Fatal(got)
	}
}
