package firefox

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/therecipe/qt/widgets"
	"go.uber.org/zap"
)

type Firefox struct {
	RootActor
	Widget *widgets.QWidget

	config    Config
	log       Logger
	runCtx    context.Context
	runCancel context.CancelFunc

	cmd    *exec.Cmd
	pid    uint32
	remote *remote
}

type Config struct {
	// Default is platform specific
	FirefoxPath string
	// Default is 49022
	DebugPort int
	// Default is .profile in current dir
	ProfilePath string
	// Default is no parent
	Parent widgets.QWidget_ITF
	// Default is zap.S()
	Log Logger
	// Default is not to log remote messages (debug level)
	LogRemoteMessages bool
}

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// Context only for startup, should have timeout or could hang forever.
func Start(ctx context.Context, config Config) (*Firefox, error) {
	// Set default config values
	var err error
	if config.FirefoxPath == "" {
		if config.FirefoxPath, err = findFirefoxPath(); err != nil {
			return nil, err
		}
	}
	if config.DebugPort == 0 {
		config.DebugPort = 49022
	}
	if config.ProfilePath == "" {
		config.ProfilePath = ".profile"
	}
	if config.Log == nil {
		config.Log = zap.S()
	}
	// Instantiate and close on any failure
	f := &Firefox{config: config, log: config.Log}
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	success := false
	defer func() {
		if !success {
			f.Close()
		}
	}()
	// Make profile path absolute
	if config.ProfilePath, err = filepath.Abs(config.ProfilePath); err != nil {
		return nil, fmt.Errorf("failed making profile path absolute: %w", err)
	}
	// Create the profile
	if err := f.prepareProfile(); err != nil {
		return nil, err
	}
	// Start firefox with the profile and remote port. Sometimes Firefox starts
	// another process and kills this one immediately, sometimes it leaves this
	// one open depending on whether started from the console or UI.
	debugPortStr := strconv.Itoa(config.DebugPort)
	cmd := exec.CommandContext(ctx, config.FirefoxPath, "-profile", config.ProfilePath,
		"-start-debugger-server", debugPortStr)
	// From console firefox starts another process, but not from UI directly
	f.log.Debugf("Running %v", cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed starting firefox: %w", err)
	}
	// Set the PID and widget
	if err = f.findAndSetPID(ctx); err != nil {
		return nil, err
	} else if err = f.findAndSetWidget(ctx); err != nil {
		return nil, err
	}
	// Start remote
	f.log.Debugf("Connecting to remote on 127.0.0.1:%v", debugPortStr)
	if f.remote, err = f.dialRemote("127.0.0.1:" + debugPortStr); err != nil {
		return nil, fmt.Errorf("failed connecting to remove: %w", err)
	}
	// Create actor manager, add root to it, and run it in background
	f.mgr = f.newActorManager()
	f.mgr.setActor("root", &f.RootActor)
	go func() {
		if err := f.mgr.run(); err != nil {
			f.log.Errorf("Actor manager failed: %v", err)
		}
	}()
	success = true
	return f, nil
}

func (f *Firefox) Close() error {
	f.runCancel()
	// Kill cmd if present, ignore error
	if f.cmd != nil {
		f.cmd.Process.Kill()
	}
	// Close remote if present, ignore error
	if f.remote != nil {
		f.remote.rw.Close()
	}
	// Kill PID
	if f.pid != 0 {
		if p, err := os.FindProcess(int(f.pid)); err != nil {
			return fmt.Errorf("failed finding firefox process to close: %w", err)
		} else if err = p.Kill(); err != nil {
			return fmt.Errorf("failed killing firefox process: %w", err)
		}
	}
	return nil
}

const defaultUserJS = `
user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.tabs.drawInTitlebar", false);
user_pref("devtools.chrome.enabled", true);
user_pref("devtools.debugger.prompt-connection", false);
user_pref("devtools.debugger.remote-enabled", true);
user_pref("toolkit.legacyUserProfileCustomizations.stylesheets", true);
`

const defaultUserChromeCSS = `
@namespace url("http://www.mozilla.org/keymaster/gatekeeper/there.is.only.xul");

#TabsToolbar {visibility: collapse;}
#navigator-toolbox {visibility: collapse;}
`

func (f *Firefox) prepareProfile() error {
	// Create the path if not there
	if err := os.MkdirAll(f.config.ProfilePath, 0755); err != nil {
		return fmt.Errorf("failed creating profile path: %w", err)
	}
	// Create the user.js file if not there
	userJSPath := filepath.Join(f.config.ProfilePath, "user.js")
	if _, err := os.Stat(userJSPath); os.IsNotExist(err) {
		f.log.Debugf("writing profile file %v", userJSPath)
		if err := ioutil.WriteFile(userJSPath, []byte(defaultUserJS), 0644); err != nil {
			return fmt.Errorf("failed writing user.js: %w", err)
		}
	}
	// Create the userChrome.css if not there
	userChromeCSSPath := filepath.Join(f.config.ProfilePath, "chrome", "userChrome.css")
	if err := os.MkdirAll(filepath.Dir(userChromeCSSPath), 0755); err != nil {
		return fmt.Errorf("failed creating chrome path: %w", err)
	} else if _, err := os.Stat(userChromeCSSPath); os.IsNotExist(err) {
		f.log.Debugf("writing profile file %v", userChromeCSSPath)
		if err := ioutil.WriteFile(userChromeCSSPath, []byte(defaultUserChromeCSS), 0644); err != nil {
			return fmt.Errorf("failed writing userChrome.css: %w", err)
		}
	}

	// That's enough. We intentionally don't create the profile via -CreateProfile
	// because Firefox would put it in the INI file. Rather, just giving the
	// directory makes it be lazily created on first use.
	return nil
}
