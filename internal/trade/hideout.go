package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"regexp"
	"time"
)

var (
	ErrNotSignedIn   = errors.New("hideout travel needs a pathofexile.com session")
	ErrHideoutDenied = errors.New("GGG did not accept the hideout request")
	hideoutTokenRE   = regexp.MustCompile(`^[A-Za-z0-9._\-]+$`)
)

// TravelToHideout is the trade site's "Travel to Hideout" for an instant
// buyout listing: GGG sends the signed-in player's game client to the
// seller's hideout. The game must be running on the same account. Like the
// trade site, a first refusal is retried once with "continue".
func (c *Client) TravelToHideout(ctx context.Context, token string) error {
	if !c.SignedIn() {
		return ErrNotSignedIn
	}
	if len(token) < 16 || len(token) > 4096 || !hideoutTokenRE.MatchString(token) {
		return errors.New("invalid hideout token")
	}
	send := func(again bool) (bool, error) {
		body := map[string]any{"token": token}
		if again {
			body["continue"] = true
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return false, err
		}
		req, err := http.NewRequest(http.MethodPost, apiBase+"/whisper", bytes.NewReader(raw))
		if err != nil {
			return false, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Origin", "https://www.pathofexile.com")
		req.Header.Set("Referer", "https://www.pathofexile.com/trade2")
		var out struct {
			Success bool `json:"success"`
		}
		if err := c.do(ctx, c.Fetch, req, &out); err != nil {
			return false, err
		}
		return out.Success, nil
	}
	ok, err := send(false)
	if err != nil || ok {
		return err
	}
	select {
	case <-time.After(500*time.Millisecond + time.Duration(rand.IntN(500))*time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	if ok, err = send(true); err != nil {
		return err
	}
	if !ok {
		return ErrHideoutDenied
	}
	return nil
}
