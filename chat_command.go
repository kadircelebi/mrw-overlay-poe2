package main

import (
	"log"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/overlay"
)

// syncChatShortcuts holds the in-game shortcuts only while the game is the
// active window, so the keys stay free for every other program: the chat
// commands (when switched on) and the one that opens the main panel.
func (s *AppService) syncChatShortcuts() {
	if s.app == nil {
		return
	}
	s.overlayMu.RLock()
	settings, capture := s.overlaySettings, s.hotkeyCapture
	s.overlayMu.RUnlock()
	var want []overlay.ChatCommand
	panelKey := ""
	if !capture && s.gameActive.Load() {
		if settings.ChatEnabled {
			want = settings.ChatCommands()
		}
		panelKey = settings.PanelHotkey
	}

	s.chatMu.Lock()
	defer s.chatMu.Unlock()
	for _, key := range s.chatKeys {
		if s.app.GlobalShortcut.IsRegistered(key) {
			_ = s.app.GlobalShortcut.Unregister(key)
		}
	}
	s.chatKeys = nil
	for _, c := range want {
		text := c.Text
		if err := s.app.GlobalShortcut.Register(c.Hotkey, func() { go s.sendChatCommand(text) }); err != nil {
			log.Printf("chat shortcut %s: %v", c.Hotkey, err)
			continue
		}
		s.chatKeys = append(s.chatKeys, c.Hotkey)
	}
	if panelKey != "" && s.tray != nil {
		err := s.app.GlobalShortcut.Register(panelKey, func() { application.InvokeAsync(s.tray.ShowWindow) })
		if err != nil {
			log.Printf("panel shortcut %s: %v", panelKey, err)
		} else {
			s.chatKeys = append(s.chatKeys, panelKey)
		}
	}
}

// setGameActive follows the foreground window. The shortcuts change only
// when the game gains or loses it; while the game is in front, its chat log
// is followed for the player who whispered last ({last}).
func (s *AppService) setGameActive(active bool, hwnd uintptr) {
	if active {
		path := overlay.GameLogPath(hwnd)
		if path != "" {
			// Kept for the filter explanation's area level (LastArea).
			s.gameLog.Store(path)
		}
		s.overlayMu.RLock()
		on := s.overlaySettings.ChatEnabled
		s.overlayMu.RUnlock()
		if on && path != "" {
			s.whispers.Follow(path)
		}
	}
	if s.gameActive.Swap(active) != active {
		s.syncChatShortcuts()
	}
}

// sendChatCommand types a line into the game's chat through the clipboard,
// as the trade tools do, and puts the player's clipboard back afterwards.
// A line naming the last whisperer is not sent while nobody has whispered.
func (s *AppService) sendChatCommand(text string) {
	if !overlay.IsGameWindow(overlay.ForegroundWindow()) {
		return
	}
	if strings.Contains(text, overlay.LastWhisperer) {
		last := s.whispers.Last()
		if last == "" {
			log.Printf("chat command %q: nobody has whispered yet", text)
			return
		}
		text = strings.ReplaceAll(text, overlay.LastWhisperer, last)
	}
	previous, hadText := s.app.Clipboard.Text()
	if !s.app.Clipboard.SetText(text) {
		return
	}
	overlay.PasteChatLine()
	time.Sleep(400 * time.Millisecond)
	if now, ok := s.app.Clipboard.Text(); ok && now == text && hadText {
		s.app.Clipboard.SetText(previous)
	}
}
