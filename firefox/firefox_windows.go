//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output firefox_gen_windows.go firefox_windows.go

package firefox

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func findFirefoxPath() (string, error) {
	// Just use fixed path for now
	const path = `C:\Program Files\Mozilla Firefox\firefox.exe`
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot find firefox at %v", path)
	}
	return path, nil
}

func findFirefoxWindowID(ctx context.Context, config Config) (uintptr, error) {
	// Continually try every so often or until context death
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-t.C:
			pid, err := getPIDListeningOnLocalhostPort(config.DebugPort)
			if err != nil {
				return 0, fmt.Errorf("failed finding PID: %w", err)
			} else if pid == 0 {
				continue
			}
			fmt.Printf("FOUND PID: %v\n", pid)
			return 0, fmt.Errorf("TODO")
		}
	}
}

func (f *Firefox) GetWindowID(ctx context.Context) (uintptr, error) {
	// Try every half second to get window, 30 times (so 15 second timeout)
	for i := 0; i < 30; i++ {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		var enumWindowsErr error
		var windowID syscall.Handle
		ok := enumWindows(syscall.NewCallback(func(handle syscall.Handle, lparam uintptr) uintptr {
			// Check the class name
			if className, err := getHandleClassName(handle); err != nil {
				enumWindowsErr = err
				return 0 // Stop enumerating
			} else if className != "MozillaWindowClass" {
				return 1
			}
			windowID = handle
			return 0
		}), 0)
		if !ok {
			return 0, fmt.Errorf("enum windows failed")
		} else if enumWindowsErr != nil {
			return 0, fmt.Errorf("failed finding window: %w", enumWindowsErr)
		} else if windowID != 0 {
			return uintptr(windowID), nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return 0, fmt.Errorf("failed finding window: timeout")
}

func firefoxPath() string { return `C:\Program Files\Mozilla Firefox\firefox.exe` }

func getHandleClassName(handle syscall.Handle) (string, error) {
	buf := make([]uint16, 100)
	n, err := getClassName(handle, &buf[0], int32(len(buf)))
	if err != nil {
		return "", fmt.Errorf("failed getting class name: %w", err)
	} else if n > int32(len(buf)) {
		return "", fmt.Errorf("invalid class name size of %v", n)
	}
	return syscall.UTF16ToString(buf[:n]), nil
}

// 0 with no error if not found
func getPIDListeningOnLocalhostPort(port int) (uint32, error) {
	// Keep trying until our buffer was large enough
	var buf []byte
	var table *mibTCPTable2
	var size uint32
	for {
		if len(buf) > 0 {
			table = (*mibTCPTable2)(unsafe.Pointer(&buf[0]))
		}
		if res := getTcpTable2(table, &size, true); res == windows.NO_ERROR {
			break
		} else if res != windows.ERROR_INSUFFICIENT_BUFFER {
			return 0, res
		}
		buf = make([]byte, size)
	}
	// Now that we have the table, look for the 127.x.x.x w/ the port
	numSize := int(unsafe.Sizeof(table.numEntries))
	rowSize := int(unsafe.Sizeof(table.table))
	for i := 0; i < int(table.numEntries); i++ {
		row := (*mibTCPRow2)(unsafe.Pointer(&buf[numSize+(i*rowSize)]))
		if row.localAddr&255 == 127 && int(syscall.Ntohs(uint16(row.localPort))) == port {
			return row.owningPID, nil
		}
	}
	return 0, nil
}

// Windows API calls

type mibTCPTable2 struct {
	numEntries uint32
	table      [1]mibTCPRow2
}

type mibTCPRow2 struct {
	state        uint32
	localAddr    uint32
	localPort    uint32
	remoteAddr   uint32
	remotePort   uint32
	owningPID    uint32
	offloadState uint32
}

//sys	findWindow(className *uint16, windowName *uint16) (handle syscall.Handle, err error) = user32.FindWindowW
//sys enumWindows(lpEnumFunc uintptr, lParam uintptr) (ok bool) = user32.EnumWindows
//sys getWindowThreadProcessID(handle syscall.Handle, processID *uint32) (threadID uint32) = user32.GetWindowThreadProcessId
//sys getClassName(handle syscall.Handle, className *uint16, classNameMax int32) (classNameLen int32, err error) = user32.GetClassNameW
//sys getTcpTable2(tcpTable *mibTCPTable2, sizePointer *uint32, order bool) (res syscall.Errno) = iphlpapi.GetTcpTable2
