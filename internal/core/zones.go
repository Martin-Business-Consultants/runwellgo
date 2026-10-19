package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"
)

// Time zones: stored in UTC, shown in the install's zone (Settings ›
// Address, else TIME_ZONE, else Eastern). A client may have its own; its
// portal, approval pages and emails use it. Zones are kept by their Rails
// names, as the Rails core kept them; an IANA name is taken and stored as
// its Rails name when it has one.

const defaultZone = "Eastern Time (US & Canada)"

// normalizeZone is a zone as stored, and whether it's one we know; "" is
// none (the install's).
func normalizeZone(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	if _, ok := railsZones[s]; ok {
		return s, true
	}
	for name, iana := range railsZones {
		if iana == s {
			return name, true
		}
	}
	if _, err := time.LoadLocation(s); err == nil && s != "Local" {
		return s, true
	}
	return "", false
}

// location is a stored zone's location; UTC when it can't be loaded.
func location(zone string) *time.Location {
	if iana, ok := railsZones[zone]; ok {
		zone = iana
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// zoneName is the install's zone: Settings › Address, else TIME_ZONE, else
// Eastern.
func (a *App) zoneName(ctx context.Context) string {
	if s, err := a.settings(ctx); err == nil && s.TimeZone != "" {
		return s.TimeZone
	}
	if tz, ok := normalizeZone(os.Getenv("TIME_ZONE")); ok && tz != "" {
		return tz
	}
	return defaultZone
}

// zone is the install's location.
func (a *App) zone(ctx context.Context) *time.Location { return location(a.zoneName(ctx)) }

// today is the date in the install's zone, YYYY-MM-DD.
func (a *App) today(ctx context.Context) string {
	return time.Now().In(a.zone(ctx)).Format("2006-01-02")
}

func init() {
	routeSets = append(routeSets, func(a *App, mux *http.ServeMux) {
		a.handle(mux, "GET /api/time_zones", a.listTimeZones)
	})
}

// listTimeZones are the zones a client or the install may be in, by their
// Rails names, as pickers offer them ("(GMT-08:00) Pacific Time (US & Canada)").
func (a *App) listTimeZones(c *Ctx) error {
	type zone struct {
		name   string
		offset int
	}
	now := time.Now()
	list := make([]zone, 0, len(railsZones))
	for name, iana := range railsZones {
		_, off := now.In(location(iana)).Zone()
		list = append(list, zone{name, off})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].offset != list[j].offset {
			return list[i].offset < list[j].offset
		}
		return list[i].name < list[j].name
	})
	out := make([]Obj, len(list))
	for i, z := range list {
		sign, off := "+", z.offset
		if off < 0 {
			sign, off = "-", -off
		}
		out[i] = O("name", z.name, "label", fmt.Sprintf("(GMT%s%02d:%02d) %s", sign, off/3600, off%3600/60, z.name))
	}
	return c.JSON(200, O("zones", out))
}
