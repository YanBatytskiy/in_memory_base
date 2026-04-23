package concurrency

import "sync"

// WithLock runs action while holding mutex. It is a convenience wrapper
// around mutex.Lock / defer mutex.Unlock that saves two lines and avoids
// forgetting the defer. Nil actions are a no-op.
func WithLock(mutex sync.Locker, action func()) {
	if action == nil {
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	action()
}
