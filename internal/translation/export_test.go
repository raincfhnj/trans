package translation

// Remembered reports how many sentences the store holds, so a test can show it
// stays bounded.
func Remembered(translator Translator) int {
	store, ok := translator.(*segmented)
	if !ok {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.known)
}

// Waiting reports how many translations the store is waiting on right now, so a
// test can tell a caller that joined a call already in flight from one that
// started its own: the first leaves this at one, the second makes it two.
func Waiting(translator Translator) int {
	store, ok := translator.(*segmented)
	if !ok {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.inflight)
}

// Sharing reports how many callers are waiting on the translations in flight,
// which is one per request sent and more where a caller joined one: it is what
// a test waits on when it needs to know a second caller has arrived and is
// sharing, rather than sleeping for long enough that it probably has.
func Sharing(translator Translator) int {
	store, ok := translator.(*segmented)
	if !ok {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	sharing := 0
	for _, running := range store.inflight {
		sharing += running.waiters
	}
	return sharing
}
