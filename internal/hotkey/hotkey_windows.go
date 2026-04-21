//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Win32 constants.
const (
	whMouseLL     = 14
	wmHotKey      = 0x0312
	wmXButtonDown = 0x020B
	wmMButtonDown = 0x0207
	wmQuit        = 0x0012

	modAlt      uint32 = 0x0001
	modControl  uint32 = 0x0002
	modShift    uint32 = 0x0004
	modWin      uint32 = 0x0008
	modNoRepeat uint32 = 0x4000

	voiceHotkeyID = uintptr(9821) // arbitrary non-conflicting ID
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
)

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

type msllHookStruct struct {
	PtX, PtY    int32
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// Config describes the user's hotkey binding.
type Config struct {
	Type      string   `json:"type"`      // "keyboard" or "mouse"
	Modifiers []string `json:"modifiers"` // ["ctrl","shift","alt","win"]
	Key       string   `json:"key"`       // "Space", "F1", "A" …
	Button    int      `json:"button"`    // 3=middle, 4=XButton1, 5=XButton2
}

// DefaultConfig is Ctrl+Shift+Space — the previous hardcoded shortcut.
func DefaultConfig() Config {
	return Config{
		Type:      "keyboard",
		Modifiers: []string{"ctrl", "shift"},
		Key:       "Space",
	}
}

// Label returns a human-readable description of the config.
func (c Config) Label() string {
	if c.Type == "mouse" {
		switch c.Button {
		case 3:
			return "Mouse Middle Button"
		case 4:
			return "Mouse Button 4 (Back)"
		case 5:
			return "Mouse Button 5 (Forward)"
		default:
			return fmt.Sprintf("Mouse Button %d", c.Button)
		}
	}
	parts := make([]string, 0, len(c.Modifiers)+1)
	for _, m := range c.Modifiers {
		switch strings.ToLower(m) {
		case "ctrl":
			parts = append(parts, "Ctrl")
		case "shift":
			parts = append(parts, "Shift")
		case "alt":
			parts = append(parts, "Alt")
		case "win":
			parts = append(parts, "Win")
		}
	}
	parts = append(parts, c.Key)
	return strings.Join(parts, "+")
}

// Manager owns one global hotkey/mouse binding and a Win32 message-loop goroutine.
type Manager struct {
	mu      sync.Mutex
	trigger func()
	cfg     Config

	quit          chan struct{}
	done          chan struct{}
	tid           uint32  // Win32 thread ID of the message-loop goroutine
	mouseCallback uintptr // kept to prevent GC
}

// New creates a Manager that calls trigger() when the hotkey fires.
func New(trigger func()) *Manager {
	return &Manager{trigger: trigger}
}

// Update unregisters the previous binding (if any) and registers the new one.
func (m *Manager) Update(cfg Config) error {
	m.Stop()
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return m.start()
}

// Stop unregisters the current binding and waits for the loop goroutine to exit.
func (m *Manager) Stop() {
	m.mu.Lock()
	quit := m.quit
	done := m.done
	tid := m.tid
	m.quit = nil
	m.done = nil
	m.tid = 0
	m.mu.Unlock()

	if quit == nil {
		return
	}
	close(quit)
	if tid != 0 {
		procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
	}
	if done != nil {
		<-done
	}
}

func (m *Manager) start() error {
	m.mu.Lock()
	cfg := m.cfg
	quit := make(chan struct{})
	done := make(chan struct{})
	m.quit = quit
	m.done = done
	m.mu.Unlock()

	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		tidVal, _, _ := procGetCurrentThreadId.Call()
		m.mu.Lock()
		m.tid = uint32(tidVal)
		m.mu.Unlock()

		var hookHandle uintptr
		var registered bool

		if cfg.Type == "mouse" {
			// The callback closes over hookHandle; it won't be called until
			// messages are pumped, so hookHandle will be set by then.
			var cb uintptr
			cb = syscall.NewCallback(func(nCode, wParam, lParam uintptr) uintptr {
				if int32(nCode) >= 0 {
					m.handleMouseMsg(wParam, lParam)
				}
				r, _, _ := procCallNextHookEx.Call(hookHandle, nCode, wParam, lParam)
				return r
			})
			// Prevent GC from collecting the callback.
			m.mu.Lock()
			m.mouseCallback = cb
			m.mu.Unlock()

			h, _, err := procSetWindowsHookExW.Call(whMouseLL, cb, 0, 0)
			if h == 0 {
				ready <- fmt.Errorf("SetWindowsHookEx: %w", err)
				return
			}
			hookHandle = h
		} else {
			mods := modifiersToFlags(cfg.Modifiers)
			vk := keyNameToVK(cfg.Key)
			if vk == 0 {
				ready <- fmt.Errorf("unknown key %q — try a letter, F1–F12, Space, etc.", cfg.Key)
				return
			}
			r, _, err := procRegisterHotKey.Call(0, voiceHotkeyID, uintptr(mods|modNoRepeat), uintptr(vk))
			if r == 0 {
				ready <- fmt.Errorf("RegisterHotKey failed (shortcut may be in use by another app): %w", err)
				return
			}
			registered = true
		}

		ready <- nil

		var m2 winMsg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m2)), 0, 0, 0)
			if r == 0 || int32(r) < 0 {
				break // WM_QUIT or error
			}
			select {
			case <-quit:
				goto cleanup
			default:
			}
			if m2.Message == wmHotKey && m2.WParam == voiceHotkeyID {
				go m.trigger()
			}
		}

	cleanup:
		if registered {
			procUnregisterHotKey.Call(0, voiceHotkeyID)
		}
		if hookHandle != 0 {
			procUnhookWindowsHookEx.Call(hookHandle)
		}
	}()

	return <-ready
}

// handleMouseMsg processes a WH_MOUSE_LL hook message.
// lParam is a Windows OS pointer to MSLLHOOKSTRUCT — it is not a Go heap
// pointer and will not be moved by the GC, so the uintptr→unsafe.Pointer
// conversion below is safe. //go:nocheckptr suppresses the false-positive
// from the unsafeptr analyser.
//
//go:nocheckptr
func (m *Manager) handleMouseMsg(wParam, lParam uintptr) {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()
	switch uint32(wParam) {
	case wmMButtonDown:
		if cfg.Button == 3 {
			go m.trigger()
		}
	case wmXButtonDown:
		hs := (*msllHookStruct)(unsafe.Pointer(lParam))
		hi := int((hs.MouseData >> 16) & 0xFFFF)
		if (hi == 1 && cfg.Button == 4) || (hi == 2 && cfg.Button == 5) {
			go m.trigger()
		}
	}
}

func modifiersToFlags(mods []string) uint32 {
	var f uint32
	for _, mod := range mods {
		switch strings.ToLower(mod) {
		case "ctrl":
			f |= modControl
		case "shift":
			f |= modShift
		case "alt":
			f |= modAlt
		case "win":
			f |= modWin
		}
	}
	return f
}

// keyNameToVK maps a human-readable key name to a Win32 virtual key code.
func keyNameToVK(name string) uint32 {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SPACE":
		return 0x20
	case "ENTER":
		return 0x0D
	case "TAB":
		return 0x09
	case "ESCAPE", "ESC":
		return 0x1B
	case "DELETE", "DEL":
		return 0x2E
	case "INSERT", "INS":
		return 0x2D
	case "HOME":
		return 0x24
	case "END":
		return 0x23
	case "PAGEUP":
		return 0x21
	case "PAGEDOWN":
		return 0x22
	case "BACKSPACE":
		return 0x08
	case "UP":
		return 0x26
	case "DOWN":
		return 0x28
	case "LEFT":
		return 0x25
	case "RIGHT":
		return 0x27
	case "F1":
		return 0x70
	case "F2":
		return 0x71
	case "F3":
		return 0x72
	case "F4":
		return 0x73
	case "F5":
		return 0x74
	case "F6":
		return 0x75
	case "F7":
		return 0x76
	case "F8":
		return 0x77
	case "F9":
		return 0x78
	case "F10":
		return 0x79
	case "F11":
		return 0x7A
	case "F12":
		return 0x7B
	default:
		u := strings.ToUpper(name)
		if len(u) == 1 {
			c := u[0]
			if c >= 'A' && c <= 'Z' {
				return uint32(c)
			}
			if c >= '0' && c <= '9' {
				return uint32(c)
			}
		}
		return 0
	}
}
