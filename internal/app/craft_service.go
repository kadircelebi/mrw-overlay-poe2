package app

import (
	"errors"
	"time"

	"poe2filter/internal/overlay"
	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
)

// CraftPrices exposes only public currency quotes. Past operations keep their
// own price snapshot in the craft ledger when this list is refreshed.
type CraftPrices struct {
	League      string                 `json:"league"`
	GeneratedAt string                 `json:"generated_at"`
	Currency    []prices.CurrencyPrice `json:"currency"`
}

func (s *AppService) GetCraftPrices() CraftPrices {
	out := CraftPrices{Currency: []prices.CurrencyPrice{}}
	if s.eng == nil {
		return out
	}
	snap := s.eng.Prices()
	if snap == nil {
		return out
	}
	out.League = snap.League
	out.GeneratedAt = snap.GeneratedAt.Format(time.RFC3339)
	out.Currency = append(out.Currency, snap.Currency...)
	// The list may omit exchange currencies that are represented by Rates.
	for _, name := range []string{"Exalted Orb", "Divine Orb", "Chaos Orb"} {
		q := overlay.QuoteCurrency(snap, name)
		if q.Found {
			out.Currency = append(out.Currency, prices.CurrencyPrice{Name: name, ValueEx: q.ValueEx, Category: "Currency"})
		}
	}
	return out
}

// craftClasses are the item classes the craft page offers (its
// data/classes.json); PoE2DB has real modifier weights for these.
var craftClasses = map[string]bool{
	"Gloves": true, "Boots": true, "Helmets": true, "Body Armours": true,
	"Shields": true, "Bucklers": true, "Foci": true, "Quivers": true,
	"Amulets": true, "Rings": true, "Belts": true,
	"Bows": true, "Crossbows": true, "One Hand Maces": true, "Two Hand Maces": true,
	"Quarterstaves": true, "Spears": true, "Talismans": true, "Sceptres": true,
	"Staves": true, "Wands": true, "Jewels": true,
}

// ParseCraftText parses a synthetic item without replacing the captured item.
func (s *AppService) ParseCraftText(raw string) (overlay.Snapshot, error) {
	if len(raw) > 16000 {
		return overlay.Snapshot{}, errors.New("craft item is too large")
	}
	catalog, err := s.GetTradeCatalog()
	if err != nil {
		return overlay.Snapshot{}, err
	}
	item, err := overlay.ParseItemWith(raw, catalog, overlay.ParseOptions{SignedIn: s.overlayClient.SignedIn()})
	if err != nil {
		return overlay.Snapshot{}, err
	}
	if !craftClasses[item.Class] {
		return overlay.Snapshot{}, errors.New("this item class cannot be crafted here")
	}
	// Without a picked base the craft names the item class in the base's
	// place. Leaving BaseType empty then makes the market search the class
	// rather than invent a base; a real base is searched as itself.
	if item.BaseType == item.Class {
		item.BaseType = ""
	}
	return overlay.Snapshot{Item: &item}, nil
}

func (s *AppService) GetMarketSnapshot() overlay.Snapshot {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	if s.marketSnapshot != nil {
		return *s.marketSnapshot
	}
	return s.overlaySnapshot
}

func (s *AppService) ShowCraftMarketWithQuery(raw string, in trade.EvaluateRequest) error {
	snap, err := s.ParseCraftText(raw)
	if err != nil {
		return err
	}
	s.overlayMu.Lock()
	s.marketSnapshot, s.overlayDraft = &snap, in
	s.overlayMu.Unlock()
	if s.marketWindow != nil {
		s.marketWindow.EmitEvent("overlay-query", in)
		s.showMarketWindow()
	}
	return nil
}

func (s *AppService) ShowCraft() {
	if s.craftWindow == nil {
		return
	}
	s.confineWindows()
	// A window the player made full screen stays so; only a normal one is
	// placed on the game's screen at its usual size.
	if screen := s.anchorScreen(); screen != nil {
		s.overlayMu.RLock()
		settings := s.overlaySettings
		s.overlayMu.RUnlock()
		scale := craftScaleFor(settings, s.windowArea())
		s.setWindowZoom(s.craftWindow, scale)
		if !s.craftWindow.IsMaximised() {
			s.craftWindow.SetScreen(screen)
			bounds := screen.WorkArea
			bounds.Width = min(int(craftWidth*scale), bounds.Width)
			bounds.Height = min(int(craftHeight*scale), bounds.Height)
			s.craftWindow.SetBounds(bounds)
			overlay.PlaceInGame(uintptr(s.craftWindow.NativeWindow()), false)
		}
	}
	s.craftWindow.Show()
	s.craftWindow.Focus()
}

// CraftImport is the copied item the craft page turns into a draft.
type CraftImport struct {
	Raw       string `json:"raw"`
	Class     string `json:"class"`
	Rarity    string `json:"rarity"`
	BaseType  string `json:"baseType"`
	ItemLevel int    `json:"itemLevel"`
}

// ShowCraftFromOverlay opens the craft window with the item last copied for
// the price check, so its affixes can be tried further.
func (s *AppService) ShowCraftFromOverlay() {
	s.overlayMu.RLock()
	item := s.overlaySnapshot.Item
	s.overlayMu.RUnlock()
	if item != nil && s.craftWindow != nil {
		s.craftWindow.EmitEvent("craft-import", CraftImport{Raw: item.Raw, Class: item.Class,
			Rarity: item.Rarity, BaseType: item.BaseType, ItemLevel: item.ItemLevel})
	}
	s.ShowCraft()
}

func (s *AppService) HideCraft() {
	if s.craftWindow != nil {
		s.craftWindow.Hide()
	}
}

func (s *AppService) toggleCraftFromHotkey() {
	if s.craftWindow == nil {
		return
	}
	if s.craftWindow.IsVisible() && s.craftWindow.IsFocused() {
		s.HideCraft()
		return
	}
	s.ShowCraft()
}
