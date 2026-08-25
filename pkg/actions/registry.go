package actions

import (
	"fmt"
	"sort"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = make(map[string]Action)
)

func Register(a Action) {
	mu.Lock()
	defer mu.Unlock()

	name := a.Metadata().Name
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("action %q already registered", name))
	}
	registry[name] = a
}

func Get(name string) (Action, bool) {
	mu.RLock()
	defer mu.RUnlock()

	a, ok := registry[name]
	return a, ok
}

func List() []Action {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]Action, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata().Name < out[j].Metadata().Name
	})
	return out
}
