package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/cretz/ffembedpoc/firefox"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"
	"go.uber.org/zap"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

var runOnMain func(func())

func funcOnMain(f func()) func() { return func() { runOnMain(f) } }

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Create app and main helper
	app := widgets.NewQApplication(len(os.Args), os.Args)
	initRunOnMain()
	// Prepare the main app
	window := widgets.NewQMainWindow(nil, 0)
	window.Resize2(800, 800)
	window.SetWindowTitle("FF Embed POC")

	// Build firefox config
	logConfig := zap.NewDevelopmentConfig()
	// Add log output if desired
	// logConfig.OutputPaths = append(logConfig.OutputPaths, "ffembedpoc.log")
	log, err := logConfig.Build()
	if err != nil {
		return err
	}
	config := firefox.Config{
		Log: log.Sugar(),
		// LogRemoteMessages: true,
	}
	// Start firefox (TODO: timeout)
	ff, err := firefox.Start(ctx, config)
	if err != nil {
		return err
	}
	defer ff.Close()
	// Create browser and start handlers
	b := newBrowser(ff, config.Log)
	if err := ff.Begin(); err != nil {
		return err
	}

	// Central widget
	layout := widgets.NewQGridLayout(nil)
	layout.AddWidget2(b.tabWidget, 0, 0, 0)
	layout.AddWidget2(ff.Widget, 1, 0, 0)
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.SetSpacing(0)
	layout.SetRowStretch(0, 0)
	layout.SetRowStretch(1, 1)
	frame := widgets.NewQFrame(nil, 0)
	frame.SetLayout(layout)
	window.SetCentralWidget(frame)

	// Show
	window.Show()

	// Handle signals
	go func() {
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)
		select {
		case <-ctx.Done():
		case <-signalCh:
			app.Exit(0)
		}
	}()

	// Run app
	app.Exec()
	return nil
}

type browser struct {
	firefox   *firefox.Firefox
	log       firefox.Logger
	tabWidget *widgets.QTabWidget
	tabs      []*browserTab
	tabsLock  sync.RWMutex
}

func newBrowser(f *firefox.Firefox, log firefox.Logger) *browser {
	b := &browser{firefox: f, log: log}
	// Create the tab widget
	b.tabWidget = widgets.NewQTabWidget(nil)
	// Add widget handlers
	b.tabWidget.ConnectCurrentChanged(func(int) {
		// Do this async since it may happen inside of update where lock is held
		go b.setSelectedFocus()
	})
	// Add listener for tab list changes
	f.TabListChangedListener.AddFunc(context.Background(), funcOnMain(b.updateTabs))
	return b
}

func (b *browser) updateTabs() {
	b.tabsLock.Lock()
	defer b.tabsLock.Unlock()
	ffTabs := b.firefox.Tabs()
	for i, tab := range ffTabs {
		if len(b.tabs) <= i {
			// New tab
			bt := newBrowserTab(b, tab)
			b.tabs = append(b.tabs, bt)
			b.tabWidget.AddTab(bt.urlEditWidget, tab.Title())
			bt.updateStateUnlocked()
		} else if b.tabs[i].tab.ID != tab.ID {
			// If there is no existing tab, insert it
			if existingIndex := b.indexOfUnlocked(tab.ID); existingIndex == -1 {
				bt := newBrowserTab(b, tab)
				b.tabs = append(b.tabs[:i], append([]*browserTab{bt}, b.tabs[i:]...)...)
				b.tabWidget.InsertTab(i, bt.urlEditWidget, tab.Title())
				bt.updateStateUnlocked()
			} else {
				// Move it
				bt := b.tabs[existingIndex]
				b.tabs = append(b.tabs[:existingIndex], b.tabs[existingIndex+1:]...)
				b.tabs = append(b.tabs[:i], append([]*browserTab{bt}, b.tabs[i:]...)...)
				b.tabWidget.TabBar().MoveTab(existingIndex, i)
			}
		}
	}
	// Remove any tabs over the length
	for len(b.tabs) > len(ffTabs) {
		b.tabs = b.tabs[:len(b.tabs)-1]
		b.tabWidget.RemoveTab(len(b.tabs))
	}
}

func (b *browser) setSelectedFocus() {
	b.tabsLock.RLock()
	defer b.tabsLock.RUnlock()
	if index := b.tabWidget.CurrentIndex(); index >= 0 && index < len(b.tabs) {
		b.tabs[index].tab.SetFocus()
	}
}

func (b *browser) indexOfUnlocked(tabID string) int {
	for i, tab := range b.tabs {
		if tab.tab.ID == tabID {
			return i
		}
	}
	return -1
}

type browserTab struct {
	*browser
	tab           *firefox.TabActor
	urlEditWidget *widgets.QLineEdit
}

func newBrowserTab(b *browser, tab *firefox.TabActor) *browserTab {
	bt := &browserTab{browser: b, tab: tab, urlEditWidget: widgets.NewQLineEdit(nil)}
	// Handle state change
	tab.StateChangedListener.AddFunc(context.Background(), funcOnMain(bt.updateState))

	// Handle favicon change
	tab.FaviconChangedListener.AddFunc(context.Background(), funcOnMain(bt.updateFavicon))
	// Handle URL change
	bt.urlEditWidget.ConnectReturnPressed(func() {
		tab.NavigateTo(bt.urlEditWidget.Text())
	})
	return bt
}

func (b *browserTab) updateState() {
	b.tabsLock.RLock()
	defer b.tabsLock.RUnlock()
	b.updateStateUnlocked()
}

func (b *browserTab) updateStateUnlocked() {
	if index := b.indexOfUnlocked(b.tab.ID); index != -1 {
		b.tabWidget.SetTabText(index, b.tab.Title())
		// If it's selected, select it
		if b.tab.Selected() {
			b.tabWidget.SetCurrentIndex(index)
		}
	}
	b.urlEditWidget.SetText(b.tab.URL())
}

func (b *browserTab) updateFavicon() {
	var icon *gui.QIcon
	if data := b.tab.Favicon(); len(data) == 0 {
		icon = gui.NewQIcon()
	} else {
		pixmap := gui.NewQPixmap()
		// TODO: This fails in cgo-less, ref https://github.com/therecipe/qt/issues/1193
		if !pixmap.LoadFromData(data, uint(len(data)), "PNG", 0) {
			b.log.Errorf("Failed loading favicon PNG")
			icon = gui.NewQIcon()
		} else {
			icon = gui.NewQIcon2(pixmap)
		}
	}
	b.tabsLock.RLock()
	defer b.tabsLock.RUnlock()
	if index := b.indexOfUnlocked(b.tab.ID); index != -1 {
		b.tabWidget.SetTabIcon(index, icon)
	}
}
