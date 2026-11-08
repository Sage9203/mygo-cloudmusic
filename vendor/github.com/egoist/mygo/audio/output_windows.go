//go:build windows && (amd64 || arm64)

package audio

import (
	"errors"
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"
)

// Windows plays through WASAPI in shared mode on the default output,
// opened again when the default changes or the device goes. Every call
// runs on the output's thread, locked, in COM's multithreaded apartment.

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

var (
	clsidMMDeviceEnumerator   = guid{0xbcde0395, 0xe52f, 0x467c, [8]byte{0x8e, 0x3d, 0xc4, 0x57, 0x92, 0x91, 0x69, 0x2e}}
	iidIMMDeviceEnumerator    = guid{0xa95664d2, 0x9614, 0x4f35, [8]byte{0xa7, 0x46, 0xde, 0x8d, 0xb6, 0x36, 0x17, 0xe6}}
	iidIAudioClient           = guid{0x1cb9ad4c, 0xdbfa, 0x4c32, [8]byte{0xb1, 0x78, 0xc2, 0xf5, 0x68, 0xa7, 0x03, 0xb2}}
	iidIAudioRenderClient     = guid{0xf294acfc, 0x3146, 0x4483, [8]byte{0xa7, 0xbf, 0xad, 0xdc, 0xa7, 0xc2, 0x60, 0xe2}}
	ksDataFormatSubtypeFloat  = guid{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	ole32                     = syscall.NewLazyDLL("ole32.dll")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance      = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree         = ole32.NewProc("CoTaskMemFree")
	procCreateEventW          = kernel32.NewProc("CreateEventW")
	procWaitForSingleObject   = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle           = kernel32.NewProc("CloseHandle")
	comInitialized            bool
	errDefaultDeviceChanged   = errors.New("audio: the default output changed")
	errAudioClientUnresponded = errors.New("audio: the audio client stopped asking for frames")
)

type waveFormatExtensible struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	size           uint16
	validBits      uint16
	channelMask    uint32
	subFormat      guid
}

const (
	clsctxAll                     = 0x17
	coinitMultithreaded           = 0
	eRender                       = 0
	eConsole                      = 0
	waveFormatExtensibleTag       = 0xfffe
	speakerFrontLeft              = 0x1
	speakerFrontRight             = 0x2
	audclntShareModeShared        = 0
	audclntStreamFlagsEventCB     = 0x00040000
	audclntStreamFlagsNoPersist   = 0x00080000
	audclntStreamFlagsAutoConvert = 0x80000000
	audclntStreamFlagsSRCQuality  = 0x08000000
	waitObject0                   = 0
	windowsFrames                 = sampleRate / 100 // per write: 10 ms
)

// comCall calls method i of the COM interface obj. Pointers converted to
// uintptr in the call stay valid until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func comCall(obj uintptr, i int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(native(obj))
	fn := *(*uintptr)(unsafe.Add(native(vtbl), i*int(unsafe.Sizeof(uintptr(0)))))
	var a [16]uintptr
	a[0] = obj
	n := copy(a[1:], args)
	r, _, _ := syscall.SyscallN(fn, a[:n+1]...)
	return r
}

// native converts a native address to a pointer.
func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hresult(what string, hr uintptr) error {
	return fmt.Errorf("audio: %s failed (HRESULT %#08x)", what, uint32(hr))
}

func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

type wasapiDevice struct {
	enumerator, client, render uintptr
	event                      uintptr
	format                     waveFormatExtensible
	exclusive                  bool
	frames                     uint32 // the client's buffer
	id                         string // the device's endpoint ID
	checked                    time.Time
	mixStatus                  string
}

// defaultID returns the endpoint ID of the default output.
func defaultID(enumerator uintptr) (string, error) {
	var dev uintptr
	if hr := comCall(enumerator, 4, eRender, eConsole, uintptr(unsafe.Pointer(&dev))); failed(hr) {
		if uint32(hr) == 0x80070490 { // E_NOTFOUND
			return "", errors.New("audio: no audio output")
		}
		return "", hresult("GetDefaultAudioEndpoint", hr)
	}
	defer release(dev)
	var p *uint16
	if hr := comCall(dev, 5, uintptr(unsafe.Pointer(&p))); failed(hr) {
		return "", hresult("IMMDevice.GetId", hr)
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n)), nil
}

func openDevice(ahead time.Duration, format outputFormat) (device, int, error) {
	if !comInitialized {
		// S_FALSE, already initialized, succeeds as well.
		if hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded); failed(hr) {
			return nil, 0, hresult("CoInitializeEx", hr)
		}
		comInitialized = true
	}
	d := &wasapiDevice{}
	err := d.open(ahead, format)
	var fallback error
	if err != nil && format.exclusive {
		fallback = err
		d.close()
		format.exclusive = false
		err = d.open(ahead, format)
	}
	if err != nil {
		d.close()
		return nil, 0, err
	}
	setOutputStatus(format.rate, int(d.format.validBits), d.exclusive, fallback)
	errMu.Lock()
	outputMixStatus = d.mixStatus
	errMu.Unlock()
	return d, min(format.rate/100, int(d.frames)), nil
}

func (d *wasapiDevice) open(ahead time.Duration, format outputFormat) error {
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&d.enumerator))); failed(hr) {
		return hresult("CoCreateInstance(MMDeviceEnumerator)", hr)
	}
	var dev uintptr
	if hr := comCall(d.enumerator, 4, eRender, eConsole, uintptr(unsafe.Pointer(&dev))); failed(hr) {
		if uint32(hr) == 0x80070490 { // E_NOTFOUND
			return errors.New("audio: no audio output")
		}
		return hresult("GetDefaultAudioEndpoint", hr)
	}
	defer release(dev)
	id, err := defaultID(d.enumerator)
	if err != nil {
		return err
	}
	d.id = id
	if hr := comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&d.client))); failed(hr) {
		return hresult("IMMDevice.Activate", hr)
	}
	d.mixStatus = readMixFormat(d.client)
	f := waveFormatExtensible{
		formatTag:      waveFormatExtensibleTag,
		channels:       channels,
		samplesPerSec:  uint32(format.rate),
		avgBytesPerSec: uint32(format.rate * channels * 4),
		blockAlign:     channels * 4,
		bitsPerSample:  32,
		size:           22,
		validBits:      32,
		channelMask:    speakerFrontLeft | speakerFrontRight,
		subFormat:      ksDataFormatSubtypeFloat,
	}
	// Exclusive formats are tested with the driver before initialization.
	// Timer-driven exclusive output uses GetCurrentPadding, like shared mode,
	// rather than event mode's full-buffer packets and alignment constraints.
	mode := uintptr(audclntShareModeShared)
	if format.exclusive {
		var ok bool
		for _, candidate := range exclusiveFormats(format.rate) {
			if hr := comCall(d.client, 7, 1, uintptr(unsafe.Pointer(&candidate)), 0); hr == 0 {
				f, ok = candidate, true
				break
			}
		}
		if !ok {
			return fmt.Errorf("设备不支持 %g kHz 独占格式", float64(format.rate)/1000)
		}
		mode = 1
	}
	d.format, d.exclusive = f, format.exclusive
	// Shared mode may convert after receiving this native-rate stream.
	duration := uintptr(ahead / 100) // in 100 ns
	flags := uintptr(audclntStreamFlagsEventCB | audclntStreamFlagsNoPersist | audclntStreamFlagsAutoConvert | audclntStreamFlagsSRCQuality)
	if format.exclusive {
		flags = 0
	}
	if hr := comCall(d.client, 3, mode, flags, duration, 0, uintptr(unsafe.Pointer(&f)), 0); failed(hr) {
		return hresult("IAudioClient.Initialize", hr)
	}
	if hr := comCall(d.client, 4, uintptr(unsafe.Pointer(&d.frames))); failed(hr) {
		return hresult("IAudioClient.GetBufferSize", hr)
	}
	if hr := comCall(d.client, 14, uintptr(unsafe.Pointer(&iidIAudioRenderClient)), uintptr(unsafe.Pointer(&d.render))); failed(hr) {
		return hresult("IAudioClient.GetService", hr)
	}
	if !format.exclusive {
		ev, _, err := procCreateEventW.Call(0, 0, 0, 0)
		if ev == 0 {
			return fmt.Errorf("audio: CreateEvent failed: %v", err)
		}
		d.event = ev
		if hr := comCall(d.client, 13, d.event); failed(hr) {
			return hresult("IAudioClient.SetEventHandle", hr)
		}
	}
	if hr := comCall(d.client, 10); failed(hr) {
		return hresult("IAudioClient.Start", hr)
	}
	d.checked = time.Now()
	return nil
}

func (d *wasapiDevice) write(samples []float32) error {
	n := uint32(len(samples) / channels)
	deadline := time.Now().Add(time.Second)
	for {
		var padding uint32
		if hr := comCall(d.client, 6, uintptr(unsafe.Pointer(&padding))); failed(hr) {
			return hresult("IAudioClient.GetCurrentPadding", hr)
		}
		if padding <= d.frames && d.frames-padding >= n {
			break
		}
		if time.Now().After(deadline) {
			return errAudioClientUnresponded
		}
		if d.event != 0 {
			procWaitForSingleObject.Call(d.event, 100)
		} else {
			time.Sleep(2 * time.Millisecond)
		}
	}
	var p unsafe.Pointer
	if hr := comCall(d.render, 3, uintptr(n), uintptr(unsafe.Pointer(&p))); failed(hr) {
		return hresult("IAudioRenderClient.GetBuffer", hr)
	}
	encodeOutput(unsafe.Slice((*byte)(p), len(samples)*int(d.format.bitsPerSample)/8), samples, d.format)
	if hr := comCall(d.render, 4, uintptr(n), 0); failed(hr) {
		return hresult("IAudioRenderClient.ReleaseBuffer", hr)
	}
	// The default output changing (headphones plugged in, a choice in
	// Settings) moves a shared stream nowhere: open the new one.
	if time.Since(d.checked) > 500*time.Millisecond {
		d.checked = time.Now()
		if id, err := defaultID(d.enumerator); err != nil || id != d.id {
			return errDefaultDeviceChanged
		}
	}
	return nil
}

// drain lets submitted samples finish before a normal format change stops the
// client. A removed or unresponsive device must not hold up the output thread.
func (d *wasapiDevice) drain() {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var padding uint32
		if hr := comCall(d.client, 6, uintptr(unsafe.Pointer(&padding))); failed(hr) || padding == 0 {
			return
		}
		if d.event != 0 {
			procWaitForSingleObject.Call(d.event, 10)
		} else {
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func (d *wasapiDevice) close() {
	if d.client != 0 {
		comCall(d.client, 11)
	}
	release(d.render)
	release(d.client)
	release(d.enumerator)
	d.render, d.client, d.enumerator = 0, 0, 0
	if d.event != 0 {
		procCloseHandle.Call(d.event)
		d.event = 0
	}
}

// PCM candidates retain at least 24 valid bits when supported. A 16-bit-only
// device is accepted and the actual depth is shown, never labelled 24-bit.
func exclusiveFormats(rate int) []waveFormatExtensible {
	pcm := guid{0x00000001, 0, 0x0010, [8]byte{0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}
	var result []waveFormatExtensible
	for _, spec := range []struct {
		bits, valid uint16
		subtype     guid
	}{
		{32, 32, ksDataFormatSubtypeFloat}, {32, 32, pcm}, {32, 24, pcm}, {24, 24, pcm}, {16, 16, pcm},
	} {
		align := spec.bits / 8 * channels
		result = append(result, waveFormatExtensible{waveFormatExtensibleTag, channels, uint32(rate), uint32(rate) * uint32(align), align, spec.bits, 22, spec.valid, speakerFrontLeft | speakerFrontRight, spec.subtype})
	}
	return result
}

func encodeOutput(dst []byte, samples []float32, f waveFormatExtensible) {
	if f.subFormat == ksDataFormatSubtypeFloat {
		copy(unsafe.Slice((*float32)(unsafe.Pointer(&dst[0])), len(samples)), samples)
		return
	}
	width := int(f.bitsPerSample / 8)
	shift := f.bitsPerSample - f.validBits
	scale := math.Ldexp(1, int(f.validBits)-1)
	for i, sample := range samples {
		x := math.Round(float64(sample) * scale)
		x = min(max(x, -scale), scale-1)
		v := uint32(int32(int64(x) << shift))
		for b := 0; b < width; b++ {
			dst[i*width+b] = byte(v >> (8 * b))
		}
	}
}
