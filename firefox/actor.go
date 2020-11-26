package firefox

import (
	"bytes"
	"sync"
)

type Actor interface {
	onMessage(msg *actorMessage)
}

type RootActor struct {
	TabListChangedListener EventListener

	mgr *actorManager
	// Not changed, completely rewritten
	tabs     []*TabActor
	tabsLock sync.RWMutex
}

func (r *RootActor) Begin() error {
	// Begins just with initial tab fetch
	return r.send(&actorMessage{To: "root", Type: "listTabs"})
}

// Safe for concurrent use but don't mutate result
func (r *RootActor) Tabs() []*TabActor {
	r.tabsLock.RLock()
	r.tabsLock.RUnlock()
	return r.tabs
}

func (r *RootActor) send(msg *actorMessage) error {
	if err := r.mgr.firefox.remote.send(msg); err != nil {
		r.mgr.firefox.log.Errorf("failed sending: %v", err)
		return err
	}
	return nil
}

func (r *RootActor) onMessage(msg *actorMessage) {
	switch {
	case msg.Type == "tabListChanged":
		// On tab list change, we have to list the tabs
		r.send(&actorMessage{To: "root", Type: "listTabs"})
	case msg.Tabs != nil:
		// These are descriptors, only consider the list changed if there's a new
		// one or they were rearranged
		r.tabsLock.Lock()
		newTabs := make([]*TabActor, len(msg.Tabs))
		for i, msgTab := range msg.Tabs {
			// Try to find the existing tab
			var tab *TabActor
			for _, existingTab := range r.tabs {
				if existingTab.ID == msgTab.Actor {
					tab = existingTab
					break
				}
			}
			if tab == nil {
				tab = newTabActor(r, msgTab.Actor)
			}
			newTabs[i] = tab
			tab.updateFromDescriptor(msgTab)
		}
		// If any were changed (added or changes spots), copy on write
		changed := len(newTabs) != len(r.tabs)
		if !changed {
			for i, existingTab := range r.tabs {
				if existingTab.ID != newTabs[i].ID {
					changed = true
				}
			}
		}
		if changed {
			r.tabs = newTabs
			r.TabListChangedListener.Fire()
		}
		r.tabsLock.Unlock()
	}
}

func (r *RootActor) removeTab(id string) {
	r.tabsLock.Lock()
	defer r.tabsLock.Unlock()
	newTabs := make([]*TabActor, 0, len(r.tabs)-1)
	for _, tab := range r.tabs {
		if tab.ID != id {
			newTabs = append(newTabs, tab)
		}
	}
	if len(newTabs) != len(r.tabs) {
		r.tabs = newTabs
		r.TabListChangedListener.Fire()
	}
	// Trigger a re-list
	r.send(&actorMessage{To: "root", Type: "listTabs"})
}

type TabActor struct {
	ID                     string
	StateChangedListener   EventListener
	FaviconChangedListener EventListener

	root *RootActor

	// Governs fields just below it
	fieldsLock sync.RWMutex
	frameID    string
	selected   bool
	title      string
	url        string
	navigating bool

	faviconLock sync.RWMutex
	favicon     []byte
}

func newTabActor(root *RootActor, id string) *TabActor {
	tab := &TabActor{ID: id, root: root}
	// Set myself on the ID
	root.mgr.setActor(tab.ID, tab)
	return tab
}

func (t *TabActor) Selected() bool {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	return t.selected
}

func (t *TabActor) Title() string {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	return t.title
}

func (t *TabActor) URL() string {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	return t.url
}

func (t *TabActor) Navigating() bool {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	return t.navigating
}

func (t *TabActor) Favicon() []byte {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	return t.favicon
}

func (t *TabActor) SetFocus() {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	t.root.send(&actorMessage{To: t.frameID, Type: "focus"})
}

func (t *TabActor) NavigateTo(url string) {
	t.fieldsLock.RLock()
	defer t.fieldsLock.RUnlock()
	t.root.send(&actorMessage{To: t.frameID, Type: "navigateTo", URL: url})
}

func (t *TabActor) onMessage(msg *actorMessage) {
	switch {
	case msg.Frame != nil:
		t.updateFromFrame(msg.Frame)
	case msg.Type == "tabNavigated":
		t.updateFromTabNavigated(msg)
	case msg.Type == "tabDetached":
		t.root.removeTab(t.ID)
	case msg.Favicon.set:
		t.updateFavicon(msg.Favicon.bytes)
	}
}

func (t *TabActor) updateFromDescriptor(msg *actorTab) {
	t.fieldsLock.Lock()
	defer t.fieldsLock.Unlock()
	// TODO: Handle missing title
	t.updateFieldsUnlocked(msg.Selected, msg.Title, msg.URL, t.navigating)
	// Ask for the new target
	t.root.send(&actorMessage{To: t.ID, Type: "getTarget"})
}

func (t *TabActor) updateFromFrame(msg *actorFrame) {
	t.fieldsLock.Lock()
	defer t.fieldsLock.Unlock()
	// Check if the frame changed
	if t.frameID != msg.Actor {
		// Remove from previous frame if there
		if t.frameID != "" {
			t.root.mgr.removeActor(t.frameID)
		}
		// Set new frame and attach
		t.frameID = msg.Actor
		t.root.mgr.setActor(t.frameID, t)
		t.root.send(&actorMessage{To: t.frameID, Type: "attach"})
	}
	// Update any other fields that may have changed
	t.updateFieldsUnlocked(t.selected, msg.Title, msg.URL, t.navigating)
}

func (t *TabActor) updateFromTabNavigated(msg *actorMessage) {
	t.fieldsLock.Lock()
	defer t.fieldsLock.Unlock()
	t.updateFieldsUnlocked(t.selected, msg.Title, msg.URL, msg.State == "start")
	// If the state is stop, ask for the favicon
	if msg.State == "stop" {
		t.root.send(&actorMessage{To: t.ID, Type: "getFavicon"})
	}
}

func (t *TabActor) updateFieldsUnlocked(selected bool, title string, url string, navigating bool) {
	// Only if changed
	changed := t.selected != selected || t.title != title || t.url != url || t.navigating != navigating
	if !changed {
		return
	}
	t.selected = selected
	t.title = title
	t.url = url
	t.navigating = navigating
	t.StateChangedListener.Fire()
}

func (t *TabActor) updateFavicon(b []byte) {
	t.faviconLock.Lock()
	defer t.faviconLock.Unlock()
	if !bytes.Equal(b, t.favicon) {
		t.favicon = b
		t.FaviconChangedListener.Fire()
	}
}
