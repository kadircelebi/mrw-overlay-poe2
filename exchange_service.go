package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"poe2filter/internal/i18n"
	"poe2filter/internal/overlay"
	"poe2filter/internal/trade"
)

// Boss keys, splinters and the like trade only on the currency exchange
// (Ange in game, /trade2/exchange on the site). The item search finds none of
// them, and when the price list does not carry one either, the overlay asks
// the exchange instead.

const exchangeCacheTTL = 2 * time.Minute

// exchangeHave are the currencies offers are asked in; each offer is turned
// into Exalted Orbs with the price list's rates.
var exchangeHave = []string{"exalted", "divine", "chaos"}

type exchangeCacheEntry struct {
	result    trade.ExchangeResult
	expiresAt time.Time
}

// ExchangeOverlay lists the bulk exchange offers for want (a trade static id).
// status is the overlay's sale-type choice; "any" asks offline sellers too.
// Identical requests within two minutes are answered from memory unless
// refresh is set.
func (s *AppService) ExchangeOverlay(want, status string, refresh bool) (trade.ExchangeResult, error) {
	want = strings.TrimSpace(want)
	if want == "" {
		return trade.ExchangeResult{}, errors.New(i18n.T("overlay.err.noExchangeItem"))
	}
	if status != "any" {
		status = "online"
	}
	league := s.eng.Config().LeagueName
	key := league + "\x00" + want + "\x00" + status

	s.exchangeMu.Lock()
	if s.exchangeCache == nil {
		s.exchangeCache = map[string]exchangeCacheEntry{}
	}
	if cached, ok := s.exchangeCache[key]; ok && !refresh && cached.expiresAt.After(time.Now()) {
		s.exchangeMu.Unlock()
		return cached.result, nil
	}
	s.exchangeMu.Unlock()

	lim := s.overlayClient.ExchangeLimiter()
	if wait := lim.NextIn(); wait > 5*time.Second {
		return trade.ExchangeResult{}, errors.New(quotaWaitMessage(lim.Status()))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := s.overlayClient.Exchange(ctx, league, want, exchangeHave, status, s.exchangeRate())
	if err != nil {
		var apiErr *trade.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.Status == 429:
			return trade.ExchangeResult{}, errors.New(quotaPenaltyMessage(lim.Status()))
		case errors.As(err, &apiErr) && apiErr.Blocked():
			return trade.ExchangeResult{}, errors.New(i18n.T("overlay.err.blocked"))
		}
		return trade.ExchangeResult{}, err
	}
	if len(result.Offers) > 20 {
		result.Offers = result.Offers[:20]
	}
	s.exchangeMu.Lock()
	s.exchangeCache[key] = exchangeCacheEntry{result: result, expiresAt: time.Now().Add(exchangeCacheTTL)}
	s.exchangeMu.Unlock()
	return result, nil
}

// exchangeRate converts a trade currency id to Exalted Orbs from the price
// snapshot (the same figures the currency card shows).
func (s *AppService) exchangeRate() func(string) float64 {
	snap := s.eng.Prices()
	names := map[string]string{"exalted": "Exalted Orb", "divine": "Divine Orb", "chaos": "Chaos Orb"}
	return func(id string) float64 {
		name, ok := names[id]
		if !ok {
			return 0
		}
		return overlay.QuoteCurrency(snap, name).ValueEx
	}
}
