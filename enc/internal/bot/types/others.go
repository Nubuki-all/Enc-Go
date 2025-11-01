package types

// NoopMutex implements the sync.Locker interface but performs no actions.
type NoopMutex struct{}

func (m *NoopMutex) Lock()   {}
func (m *NoopMutex) Unlock() {}
