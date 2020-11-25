//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output firefox_gen_windows.go firefox_windows.go

package firefox

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"
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

func (f *Firefox) findAndSetPID(ctx context.Context) error {
	// Continually try every so often or until context death
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			// Find the process listening to our debug port
			pid, err := getPIDListeningOnLocalhostPort(f.config.DebugPort)
			if err != nil {
				return fmt.Errorf("failed finding PID: %w", err)
			} else if pid == 0 {
				continue
			}
			f.log.Debugf("Found PID: %X", pid)
			f.pid = pid
			return nil
		}
	}
}

func (f *Firefox) findAndSetWidget(ctx context.Context) error {
	// Continually try every so often or until context death
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			// Find the window ID for the process
			windowID, err := getWindowIDForPIDAndClassName(f.pid, "MozillaWindowClass")
			if err != nil {
				return fmt.Errorf("failed finding window ID: %w", err)
			} else if windowID == 0 {
				continue
			}
			f.log.Debugf("Found HWND: %X", windowID)
			win := gui.QWindow_FromWinId(windowID)
			if win == nil {
				return fmt.Errorf("failed capturing window")
			}
			// Set the widget
			f.Widget = widgets.QWidget_CreateWindowContainer(win, f.config.Parent, 0)
			return nil
		}
	}
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
		res := getTcpTable2(table, &size, true)
		if res == windows.NO_ERROR {
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

// Not found is 0 with no error
func getWindowIDForPIDAndClassName(pid uint32, className string) (uintptr, error) {
	var enumWindowsErr error
	var windowID uintptr
	ok := enumWindows(syscall.NewCallback(func(handle syscall.Handle, lparam uintptr) uintptr {
		// Check the process ID
		var handlePID uint32
		getWindowThreadProcessID(handle, &handlePID)
		if handlePID != pid {
			return 1
		}
		// Check the class name
		if handleClassName, err := getHandleClassName(handle); err != nil {
			enumWindowsErr = err
			return 0 // Stop enumerating
		} else if handleClassName != className {
			return 1
		}
		windowID = uintptr(handle)
		return 1
	}), 0)
	if windowID == 0 && !ok {
		return 0, fmt.Errorf("enum windows failed")
	} else if enumWindowsErr != nil {
		return 0, fmt.Errorf("failed finding window: %w", enumWindowsErr)
	}
	return windowID, nil
}

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
