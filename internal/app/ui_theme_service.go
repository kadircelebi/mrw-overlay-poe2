package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"poe2filter/internal/uitheme"
	"strconv"
)

func (s *AppService) GetUIThemes() (uitheme.State, error) { return s.uiThemes.Get() }
func (s *AppService) CreateUITheme(sourceID, name string) (uitheme.State, error) {
	s.uiThemeMu.Lock()
	defer s.uiThemeMu.Unlock()
	return s.publishUITheme(s.uiThemes.Create(sourceID, name))
}
func (s *AppService) SaveUITheme(theme uitheme.Theme) (uitheme.State, error) {
	s.uiThemeMu.Lock()
	defer s.uiThemeMu.Unlock()
	return s.publishUITheme(s.uiThemes.Save(theme))
}
func (s *AppService) SelectUITheme(id string) (uitheme.State, error) {
	s.uiThemeMu.Lock()
	defer s.uiThemeMu.Unlock()
	return s.publishUITheme(s.uiThemes.Select(id))
}
func (s *AppService) DeleteUITheme(id string) (uitheme.State, error) {
	s.uiThemeMu.Lock()
	defer s.uiThemeMu.Unlock()
	return s.publishUITheme(s.uiThemes.Delete(id))
}

func themeBackground(state uitheme.State) application.RGBA {
	for _, t := range state.Themes {
		if t.ID == state.Selected {
			n, err := strconv.ParseUint(t.Colors["bg"][1:], 16, 24)
			if err == nil {
				return application.NewRGB(uint8(n>>16), uint8(n>>8), uint8(n))
			}
		}
	}
	return application.NewRGB(16, 18, 21)
}
func (s *AppService) uiThemeBackground() application.RGBA {
	state, _ := s.uiThemes.Get()
	return themeBackground(state)
}
func (s *AppService) publishUITheme(state uitheme.State, err error) (uitheme.State, error) {
	if err != nil {
		return state, err
	}
	if s.app != nil {
		colour := themeBackground(state)
		for _, w := range []application.Window{s.panel, s.settingsWindow, s.overlayWindow, s.marketWindow, s.craftWindow} {
			if w != nil {
				w.SetBackgroundColour(colour)
			}
		}
		s.app.Event.Emit("ui-theme", state)
	}
	return state, nil
}
