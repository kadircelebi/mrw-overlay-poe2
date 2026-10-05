package app

import "poe2filter/internal/overlay"

// Recording temporarily releases our registrations so Windows delivers even
// the existing combination to the focused input. Blur restores them.
func (s *AppService) SetHotkeyCapture(active bool) error {
	s.overlayMu.Lock()
	defer s.overlayMu.Unlock()
	if s.rebindOverlay == nil || s.hotkeyCapture == active {
		return nil
	}
	settings := s.overlaySettings
	var err error
	if active {
		err = s.rebindOverlay(settings, overlay.Settings{})
	} else {
		err = s.rebindOverlay(overlay.Settings{}, settings)
	}
	if err == nil {
		s.hotkeyCapture = active
	}
	go s.syncChatShortcuts() // takes overlayMu itself
	return err
}
