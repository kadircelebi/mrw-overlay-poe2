package app

import (
	"image"
	"log"

	"poe2filter/internal/campaign"
	"poe2filter/internal/overlay"
)

// The campaign box lies over the game while the player is in a campaign
// area and says what to watch for there, following the game's log. Off the
// campaign (maps, hideouts) it is hidden.

// The box's window size in window pixels: a note's wrapped lines must fit
// (see the guide's test). The window stays this size and the box inside it
// is only as tall as its notes; the rest is see-through and lets clicks by.
const (
	campaignBoxWidth  = 340
	campaignBoxHeight = 330
)

// CampaignStatusView is what the campaign box shows: the area the player is
// in, with its act (Area nil when it is not a campaign area).
type CampaignStatusView struct {
	On      bool           `json:"on"`
	Placing bool           `json:"placing"`
	Area    *campaign.Area `json:"area"`
	ActID   string         `json:"actId"`
	ActEN   string         `json:"actEn"`
	ActTR   string         `json:"actTr"`
}

// GetCampaignGuide returns the whole campaign, for the tracker.
func (s *AppService) GetCampaignGuide() campaign.Guide {
	return *campaign.Load()
}

// GetCampaignStatus returns what the campaign box shows.
func (s *AppService) GetCampaignStatus() CampaignStatusView {
	id, _ := s.campaignArea.Load().(string)
	v := CampaignStatusView{On: s.campaignBoxOn(), Placing: s.campaignPlacing.Load()}
	if a, act, ok := campaign.Find(id); ok {
		v.Area, v.ActID, v.ActEN, v.ActTR = &a, act.ID, act.EN, act.TR
	}
	return v
}

// SetCampaignBox shows or hides the campaign box over the game.
func (s *AppService) SetCampaignBox(on bool) error {
	next := s.GetOverlaySettings()
	next.CampaignBox = on
	_, err := s.SaveOverlaySettings(next)
	return err
}

func (s *AppService) campaignBoxOn() bool {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	return s.overlaySettings.Enabled && s.overlaySettings.CampaignBox
}

// campaignAreaEntered follows the game into an area.
func (s *AppService) campaignAreaEntered(a overlay.Area) {
	if prev, _ := s.campaignArea.Load().(string); prev == a.ID {
		return
	}
	s.campaignArea.Store(a.ID)
	s.campaignChanged()
}

// placeCampaign takes the player's area from the game's log: nothing told
// the app of it while the app was closed or the log was not followed.
func (s *AppService) placeCampaign() {
	if !s.campaignBoxOn() {
		return
	}
	if path, _ := s.gameLog.Load().(string); path != "" {
		if a, ok := overlay.CurrentArea(path); ok {
			s.campaignAreaEntered(a)
		}
	}
}

func (s *AppService) campaignChanged() {
	if s.app != nil {
		s.app.Event.Emit("campaign-status", s.GetCampaignStatus())
	}
	s.syncCampaignBox()
}

// syncCampaignBox shows the box over the game while it is the active window,
// the box is on and the player is in a campaign area, and hides it otherwise.
func (s *AppService) syncCampaignBox() {
	defer s.syncGrip("campaign")
	w := s.campaignBoxWindow
	if w == nil || s.campaignPlacing.Load() || s.boxMoving("campaign") {
		return
	}
	id, _ := s.campaignArea.Load().(string)
	_, _, known := campaign.Find(id)
	if !known || !s.campaignBoxOn() || !s.gameActive.Load() {
		if w.IsVisible() {
			w.Hide()
		}
		return
	}
	client, ok := overlay.GameClient()
	if !ok {
		return
	}
	showBox(w)
	overlay.ShowOver(uintptr(w.NativeWindow()), s.campaignBoxRect(client))
	giveFocusBack(w)
}

// campaignBoxRect is the box's place in client for the current settings.
func (s *AppService) campaignBoxRect(client image.Rectangle) image.Rectangle {
	s.overlayMu.RLock()
	st := s.overlaySettings
	s.overlayMu.RUnlock()
	dpi := 1.0
	if screen := s.anchorScreen(); screen != nil && screen.ScaleFactor > 0 {
		dpi = float64(screen.ScaleFactor)
	}
	return boxRect(client, st.CampaignCorner, st.CampaignX, st.CampaignY, dpi, campaignBoxWidth, campaignBoxHeight)
}

// PlaceCampaignBox starts placing the campaign box over the game.
func (s *AppService) PlaceCampaignBox() string {
	w := s.campaignBoxWindow
	if w == nil {
		return PlaceNoGame
	}
	client, ok := overlay.GameClient()
	if !ok {
		return PlaceNoGame
	}
	s.campaignPlacing.Store(true)
	w.SetIgnoreMouseEvents(false)
	w.Show()
	overlay.ShowOver(uintptr(w.NativeWindow()), s.campaignBoxRect(client))
	s.campaignChanged()
	return PlaceOK
}

// FinishCampaignBoxPlace ends placing; with save, the box keeps the place it
// was dragged to, as a share of the game's client area.
func (s *AppService) FinishCampaignBoxPlace(save bool) {
	w := s.campaignBoxWindow
	if w == nil || !s.campaignPlacing.Load() {
		return
	}
	if save {
		r, okWin := overlay.WindowRect(uintptr(w.NativeWindow()))
		client, okGame := overlay.GameClient()
		if okWin && okGame && client.Dx() > 0 && client.Dy() > 0 {
			next := s.GetOverlaySettings()
			next.CampaignCorner = overlay.EarningsCustom
			next.CampaignX = float64(r.Min.X-client.Min.X) / float64(client.Dx())
			next.CampaignY = float64(r.Min.Y-client.Min.Y) / float64(client.Dy())
			if _, err := s.SaveOverlaySettings(next); err != nil {
				log.Printf("campaign box: save place: %v", err)
			}
		}
	}
	w.SetIgnoreMouseEvents(true)
	s.campaignPlacing.Store(false)
	w.Hide()
	s.campaignChanged()
}
