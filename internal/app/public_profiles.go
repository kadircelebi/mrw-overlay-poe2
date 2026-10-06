package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"poe2filter/internal/engine"
	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/profileclient"
	"poe2filter/internal/publicprofile"
)

// Followed profiles are checked this often; an author's change reaches
// followers within this time (or at the next start).
const followCheckEvery = 30 * time.Minute

// PublishInfo is what the publish form needs about one profile.
type PublishInfo struct {
	// Account is the PoE account shown as the author ("" = not connected).
	Account string `json:"account"`
	// Listing is the published state, when the profile is published.
	Listing *publicprofile.Listing `json:"listing"`
	// Sounds are the groups whose own sound file is not shared.
	Sounds []publicprofile.SoundSwap `json:"sounds"`
	// Followed is true for a followed profile, which cannot be published.
	Followed bool `json:"followed"`
}

// PublicProfileTags are the labels an author may choose from.
func (s *AppService) PublicProfileTags() []string { return publicprofile.Tags }

// PublicProfileReasons are the reasons a report may give.
func (s *AppService) PublicProfileReasons() []string {
	return []string{"spam", "offensive", "broken", "other"}
}

func (s *AppService) profileCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func serverErr(err error) error {
	if err == nil {
		return nil
	}
	var e *profileclient.Error
	if errors.As(err, &e) {
		return fmt.Errorf(i18n.T("err.profileServer"), e.Code)
	}
	return fmt.Errorf(i18n.T("err.profileServer"), err)
}

// PublicProfiles lists one page of published profiles (most followed first),
// optionally searched by name/author and narrowed to a tag.
func (s *AppService) PublicProfiles(query, tag string, page int) ([]publicprofile.Listing, error) {
	ctx, cancel := s.profileCtx()
	defer cancel()
	list, err := s.profiles.List(ctx, query, tag, page)
	return list, serverErr(err)
}

// ProfilePublishInfo describes a profile for the publish form.
func (s *AppService) ProfilePublishInfo(name string) (PublishInfo, error) {
	info := PublishInfo{Account: s.session.AccountName(), Sounds: []publicprofile.SoundSwap{}}
	_, swaps, publicID, err := s.eng.SharedProfile(name)
	if err != nil {
		for _, p := range s.eng.Profiles() {
			if p.Name == name && p.Follow != nil {
				info.Followed = true
				return info, nil
			}
		}
		return info, err
	}
	if swaps != nil {
		info.Sounds = swaps
	}
	if publicID == "" {
		return info, nil
	}
	ctx, cancel := s.profileCtx()
	defer cancel()
	mine, err := s.profiles.Mine(ctx)
	if err != nil {
		return info, serverErr(err)
	}
	for i := range mine {
		if mine[i].ID == publicID {
			info.Listing = &mine[i]
		}
	}
	if info.Listing == nil {
		// Removed on the server (by its owner elsewhere, or moderation).
		_ = s.eng.SetPublicID(name, "")
	}
	return info, nil
}

// PublishProfile publishes a profile, or updates the published copy. Only
// the filter settings go; sound files fall back to built-in sounds.
func (s *AppService) PublishProfile(name string, meta publicprofile.Meta) (publicprofile.Listing, error) {
	account := s.session.AccountName()
	if account == "" {
		return publicprofile.Listing{}, errors.New(i18n.T("err.publishNoAccount"))
	}
	doc, _, publicID, err := s.eng.SharedProfile(name)
	if err != nil {
		return publicprofile.Listing{}, err
	}
	ctx, cancel := s.profileCtx()
	defer cancel()
	l, err := s.profiles.Publish(ctx, publicID, account, meta, doc)
	if publicID != "" && profileclient.IsCode(err, "not_found") {
		l, err = s.profiles.Publish(ctx, "", account, meta, doc)
	}
	if err != nil {
		return l, serverErr(err)
	}
	return l, s.eng.SetPublicID(name, l.ID)
}

// UnpublishProfile removes a profile from the server; followers keep their
// last copy.
func (s *AppService) UnpublishProfile(name string) error {
	_, _, publicID, err := s.eng.SharedProfile(name)
	if err != nil || publicID == "" {
		return err
	}
	ctx, cancel := s.profileCtx()
	defer cancel()
	if err := s.profiles.Unpublish(ctx, publicID); err != nil && !profileclient.IsCode(err, "not_found") {
		return serverErr(err)
	}
	return s.eng.SetPublicID(name, "")
}

// FollowProfile downloads a published profile, adds it as a followed
// profile and switches to it.
func (s *AppService) FollowProfile(id string) (filter.Config, error) {
	ctx, cancel := s.profileCtx()
	defer cancel()
	l, doc, err := s.profiles.Get(ctx, id)
	if err != nil {
		return s.eng.Config(), serverErr(err)
	}
	name, err := s.eng.AddFollowed(l, doc)
	if err != nil {
		return s.eng.Config(), err
	}
	if _, err := s.profiles.Follow(ctx, id, true); err != nil {
		// The profile works without the count; it is retried on the next check.
		s.eng.Logf("profile follow count: %v", err)
	} else {
		_ = s.eng.SetFollowCounted(id)
	}
	return s.SwitchProfile(name)
}

// ReportProfile flags a published profile for moderation.
func (s *AppService) ReportProfile(id, reason, note string) error {
	ctx, cancel := s.profileCtx()
	defer cancel()
	return serverErr(s.profiles.Report(ctx, id, reason, note))
}

// followLoop keeps followed profiles current, the way followed filters work
// elsewhere in PoE: the author updates, followers get it without asking.
func (s *AppService) followLoop(ctx context.Context) {
	select {
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		return
	}
	for {
		s.checkFollowed(ctx)
		select {
		case <-time.After(followCheckEvery):
		case <-ctx.Done():
			return
		}
	}
}

func (s *AppService) checkFollowed(ctx context.Context) {
	local := s.eng.FollowedVersions()
	if len(local) == 0 {
		return
	}
	ids := make([]string, 0, len(local))
	for id := range local {
		ids = append(ids, id)
	}
	cctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for _, id := range s.eng.UncountedFollows() {
		if _, err := s.profiles.Follow(cctx, id, true); err == nil || profileclient.IsCode(err, "not_found") {
			_ = s.eng.SetFollowCounted(id)
		}
	}
	remote, err := s.profiles.Versions(cctx, ids)
	if err != nil {
		s.eng.Logf("followed profiles: %v", err)
		return
	}
	changed := false
	for id, have := range local {
		var u engine.FollowedUpdate
		v, ok := remote[id]
		switch {
		case !ok:
			// Removed on the server: keep the last copy, mark it.
		case v == have:
			continue
		default:
			l, doc, err := s.profiles.Get(cctx, id)
			if err != nil {
				s.eng.Logf("followed profile %s: %v", id, err)
				continue
			}
			u = engine.FollowedUpdate{Listing: l, Document: &doc}
		}
		active, name, err := s.eng.UpdateFollowed(id, u)
		if err != nil {
			s.eng.Logf("followed profile %s: %v", id, err)
			continue
		}
		changed = true
		if active != nil {
			before := i18n.Current()
			saved, err := s.eng.SetConfig(*active)
			if err != nil {
				s.eng.Logf("followed profile %s: %v", id, err)
				continue
			}
			s.applyLanguage(saved, before)
			s.configChanged(saved)
			_ = s.eng.UpdateNow()
			if s.notify != nil {
				s.notify("followed-"+id, i18n.T("notify.followedTitle"), i18n.T("notify.followedBody", name))
			}
		}
	}
	if changed && s.app != nil {
		s.app.Event.Emit("profiles", s.eng.Profiles())
	}
}
