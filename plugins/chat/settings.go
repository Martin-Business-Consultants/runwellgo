package chat

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Each person's chat settings: the name and picture chat shows (else their
// Runwell ones), a status, the hours they're available (outside them
// they're away and nothing notifies them), what notifies them, and
// whether their tab chimes or shows browser notifications.

func settingsJSON(p profile, siteZone string) map[string]any {
	h := p.Hours
	if h.Days == nil {
		h.Days = []int{}
	}
	return map[string]any{"display_name": p.DisplayName, "status_text": p.StatusText, "status_emoji": p.StatusEmoji,
		"hours": h, "notify": p.Notify, "sound": p.Sound, "browser": p.Browser, "has_avatar": p.AvatarFileID > 0,
		"avatar_url": avatarURL(p.UserID, p), "site_zone": siteZone}
}

func showSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	p, err := profileOf(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	site, _ := api.Site(ctx)
	return ok(map[string]any{"settings": settingsJSON(p, site.TimeZone)})
}

func updateSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	body := wrapped(params(req), "settings")
	p, err := profileOf(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	errs := map[string][]string{}
	text := func(key string, most int, into *string) {
		if v, ok := body[key]; ok {
			s := strings.TrimSpace(str(v))
			if utf8.RuneCountInString(s) > most {
				errs[key] = append(errs[key], "is too long")
				return
			}
			*into = s
		}
	}
	text("display_name", 80, &p.DisplayName)
	text("status_text", 100, &p.StatusText)
	text("status_emoji", 12, &p.StatusEmoji)
	if v, ok := body["notify"]; ok {
		if !slices.Contains(notifyLevels, str(v)) {
			errs["notify"] = append(errs["notify"], "is all, mentions or none")
		} else {
			p.Notify = str(v)
		}
	}
	if v, ok := body["sound"]; ok {
		p.Sound = truthy(v)
	}
	if v, ok := body["browser"]; ok {
		p.Browser = truthy(v)
	}
	if h, ok := body["hours"].(map[string]any); ok {
		nh := hours{On: truthy(h["on"]), Start: str(h["start"]), End: str(h["end"]), Zone: str(h["zone"]), Days: []int{}}
		for _, d := range ids(h["days"]) {
			if d >= 0 && d <= 6 && !slices.Contains(nh.Days, int(d)) {
				nh.Days = append(nh.Days, int(d))
			}
		}
		if days, ok := h["days"].([]any); ok {
			for _, d := range days {
				if intOf(d) == 0 && str(d) == "0" && !slices.Contains(nh.Days, 0) {
					nh.Days = append(nh.Days, 0) // Sunday
				}
			}
		}
		slices.Sort(nh.Days)
		if _, ok := clock(nh.Start); !ok {
			errs["hours.start"] = append(errs["hours.start"], "is a time like 09:00")
		}
		if _, ok := clock(nh.End); !ok {
			errs["hours.end"] = append(errs["hours.end"], "is a time like 17:30")
		}
		if nh.Zone != "" {
			if _, err := time.LoadLocation(nh.Zone); err != nil {
				errs["hours.zone"] = append(errs["hours.zone"], "isn't a time zone this server knows")
			}
		}
		if nh.On && len(nh.Days) == 0 {
			errs["hours.days"] = append(errs["hours.days"], "needs at least one day")
		}
		p.Hours = nh
	}
	if len(errs) > 0 {
		return plugin.Invalid(errs), nil
	}
	if err := saveProfile(ctx, api, p); err != nil {
		return nil, err
	}
	if err := announce(ctx, api, me.ID); err != nil {
		return nil, err
	}
	site, _ := api.Site(ctx)
	return ok(map[string]any{"summary": "Saved your chat settings.", "settings": settingsJSON(p, site.TimeZone)})
}
