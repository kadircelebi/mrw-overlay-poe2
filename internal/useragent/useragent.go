// Package useragent is the one User-Agent every outgoing request carries.
// GGG asks tools that use its APIs to name themselves, their version and a
// way to reach the author; the repository address is that contact.
package useragent

import "sync"

// Contact is where GGG (or anyone else) can reach the author.
const Contact = "https://github.com/kadircelebi/poe2-filtre"

var (
	mu    sync.RWMutex
	value = build("MrW-POE2-Filter", "dev")
)

func build(product, version string) string {
	return product + "/" + version + " (contact: " + Contact + ")"
}

// Set names the program and its version; call it once at start.
func Set(product, version string) {
	mu.Lock()
	value = build(product, version)
	mu.Unlock()
}

// Value is the User-Agent header to send.
func Value() string {
	mu.RLock()
	defer mu.RUnlock()
	return value
}
