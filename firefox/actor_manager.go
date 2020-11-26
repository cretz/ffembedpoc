package firefox

import (
	"context"
	"sync"
)

type actorManager struct {
	firefox    *Firefox
	actors     map[string]Actor
	actorsLock sync.RWMutex
}

func (f *Firefox) newActorManager() *actorManager {
	return &actorManager{firefox: f, actors: map[string]Actor{}}
}

// Returns nil when done
func (a *actorManager) run() error {
	// Continually receive messages
	for {
		// Get next message
		var msg actorMessage
		if err := a.firefox.remote.recv(&msg); err != nil {
			if a.firefox.runCtx.Err() != nil {
				return nil
			}
			return err
		}
		a.actorsLock.RLock()
		actor := a.actors[msg.From]
		a.actorsLock.RUnlock()
		if actor != nil {
			actor.onMessage(&msg)
		}
	}
}

func (a *actorManager) setActor(id string, actor Actor) {
	a.actorsLock.Lock()
	defer a.actorsLock.Unlock()
	a.actors[id] = actor
}

func (a *actorManager) removeActor(id string) {
	a.actorsLock.Lock()
	defer a.actorsLock.Unlock()
	delete(a.actors, id)
}

type EventListener struct {
	chans     map[chan<- struct{}]struct{}
	chansLock sync.RWMutex
}

// Channel must have buffer, sends are non-blocking. Does not duplicate, just
// replaces for same chan.
func (e *EventListener) Add(ch chan<- struct{}) {
	e.chansLock.Lock()
	defer e.chansLock.Unlock()
	if e.chans == nil {
		e.chans = map[chan<- struct{}]struct{}{}
	}
	e.chans[ch] = struct{}{}
}

// Just calls Add with a new channel w/ buffer of 2
func (e *EventListener) AddFunc(ctx context.Context, fn func()) {
	ch := make(chan struct{}, 2)
	go func() {
		defer e.Remove(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				fn()
			}
		}
	}()
	e.Add(ch)
}

func (e *EventListener) Remove(ch chan<- struct{}) {
	e.chansLock.Lock()
	defer e.chansLock.Unlock()
	if e.chans != nil {
		delete(e.chans, ch)
	}
}

func (e *EventListener) Fire() {
	e.chansLock.RLock()
	defer e.chansLock.RUnlock()
	for ch := range e.chans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
