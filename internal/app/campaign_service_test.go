package app

import (
	"testing"

	"poe2filter/internal/overlay"
)

func TestCampaignStatusFollowsTheArea(t *testing.T) {
	s := &AppService{}
	if v := s.GetCampaignStatus(); v.Area != nil || v.On {
		t.Fatalf("before any area: %+v", v)
	}
	s.campaignAreaEntered(overlay.Area{ID: "G3_3", Level: 34})
	v := s.GetCampaignStatus()
	if v.Area == nil || v.Area.Name != "Jungle Ruins" || v.ActID != "act3" || v.ActEN != "Act 3" {
		t.Fatalf("in a campaign area: %+v", v)
	}
	s.campaignAreaEntered(overlay.Area{ID: "MapWaywardIsle", Level: 79})
	if v := s.GetCampaignStatus(); v.Area != nil || v.ActID != "" {
		t.Fatalf("in a map: %+v", v)
	}
}

func TestCampaignBoxNeedsTheOverlayAndItsSwitch(t *testing.T) {
	s := &AppService{}
	for _, c := range []struct {
		enabled, box, want bool
	}{{false, false, false}, {true, false, false}, {false, true, false}, {true, true, true}} {
		s.overlaySettings.Enabled, s.overlaySettings.CampaignBox = c.enabled, c.box
		if got := s.campaignBoxOn(); got != c.want {
			t.Errorf("enabled %v box %v: %v", c.enabled, c.box, got)
		}
	}
}
