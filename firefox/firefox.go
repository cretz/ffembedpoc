package firefox

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type Firefox struct {
	remote *remote
}

type Config struct {
	// Default is platform specific
	FirefoxPath string
	// Default is 49022
	DebugPort int
	// Default is .profile in current dir
	ProfilePath string
}

// Context only for startup
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
	// Make profile path absolute
	if config.ProfilePath, err = filepath.Abs(config.ProfilePath); err != nil {
		return nil, fmt.Errorf("failed making profile path absolute: %w", err)
	}
	// Create the profile
	if err := getOrCreateFirefoxProfile(config.FirefoxPath, config.ProfilePath); err != nil {
		return nil, err
	}
	// Start firefox with the profile and remote port. Note, the command exits
	// immediately because firefox starts up other processes.
	// TODO: Accept port as config and check if remote already open first
	debugPortStr := strconv.Itoa(config.DebugPort)
	cmd := exec.CommandContext(ctx, config.FirefoxPath, "-profile", config.ProfilePath,
		"-start-debugger-server", debugPortStr)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed starting firefox: %w, output: %s", err, out)
	}
	// Get the window ID
	if _, err = findFirefoxWindowID(ctx, config); err != nil {
		return nil, err
	}
	// Start remote
	remote, err := dialRemote("127.0.0.1:" + debugPortStr)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to remove: %w", err)
	}
	// Get the first message
	msg, err := remote.recv()
	if err != nil {
		return nil, fmt.Errorf("failed receiving from remote: %w", err)
	}
	fmt.Printf("MESSAGE: %v - %#v\n", msg, msg)
	// Send a message
	if err := remote.send(map[string]interface{}{"to": "root", "type": "getRoot"}); err != nil {
		return nil, fmt.Errorf("failed sending: %w", err)
	}
	// Get next message
	msg, err = remote.recv()
	if err != nil {
		return nil, fmt.Errorf("failed receiving from remote: %w", err)
	}
	fmt.Printf("MESSAGE: %v - %#v\n", msg, msg)
	time.Sleep(10 * time.Minute)
	return nil, fmt.Errorf("TODO!!")
}

func (f *Firefox) Close() error {
	// TODO: Kill
	return nil
}

const defaultUserJS = `
user_pref("devtools.chrome.enabled", true);
user_pref("devtools.debugger.remote-enabled", true);
user_pref("devtools.debugger.prompt-connection", false);
user_pref("browser.shell.checkDefaultBrowser", false);
`

func getOrCreateFirefoxProfile(firefoxPath, profilePath string) error {
	// Create the path if not there
	if err := os.MkdirAll(profilePath, 0755); err != nil {
		return fmt.Errorf("failed creating profile path: %w", err)
	}
	// Create the user.js file if not there
	userJSPath := filepath.Join(profilePath, "user.js")
	if _, err := os.Stat(userJSPath); os.IsNotExist(err) {
		if err := ioutil.WriteFile(userJSPath, []byte(defaultUserJS), 0644); err != nil {
			return fmt.Errorf("failed writing user.js: %w", err)
		}
	}
	// That's enough. We intentionally don't create the profile via -CreateProfile
	// because Firefox would put it in the INI file. Rather, just giving the
	// directory makes it be lazily created on first use.
	return nil
}
