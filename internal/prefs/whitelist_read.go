package prefs

// ListWhitelist returns a copy of whitelist entries from the store.
func (s *Store) ListWhitelist() []WhitelistEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]WhitelistEntry(nil), s.data.Whitelist...)
}
