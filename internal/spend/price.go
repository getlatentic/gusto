package spend

import (
	"slices"
	"time"
)

// Price is what a provider charges for a million tokens of each kind, in US
// dollars. Cached input and cache writes are billed at rates of their own; a
// price that leaves one out charges it as ordinary input.
type Price struct {
	InputPerMillion       float64  `json:"inputPerMillion"`
	CachedInputPerMillion *float64 `json:"cachedInputPerMillion,omitempty"`
	CacheWritePerMillion  *float64 `json:"cacheWritePerMillion,omitempty"`
	OutputPerMillion      float64  `json:"outputPerMillion"`
	// Peak is when the provider charges more. DeepSeek charges twice its base
	// rate on weekday mornings, UTC, so one call costs two amounts depending
	// on the hour it was sent.
	Peak *Peak `json:"peak,omitempty"`
}

// Peak is a multiplier and the hours it applies in.
type Peak struct {
	Multiplier float64 `json:"multiplier"`
	// Weekdays counts from Sunday as 0, as Go and cron do.
	Weekdays []time.Weekday `json:"weekdays"`
	// HoursUTC are [from, to) pairs of whole hours.
	HoursUTC [][2]int `json:"hoursUtc"`
	// Except is dates, as 2006-01-02 in UTC, the provider does not charge peak
	// on, such as its public holidays. The provider publishes them and the
	// operator writes them here.
	Except []string `json:"except,omitempty"`
}

// at is the multiplier for a call sent at this moment.
func (p *Peak) at(sent time.Time) float64 {
	if p == nil {
		return 1
	}
	sent = sent.UTC()
	if !slices.Contains(p.Weekdays, sent.Weekday()) || slices.Contains(p.Except, sent.Format("2006-01-02")) {
		return 1
	}
	for _, hours := range p.HoursUTC {
		if sent.Hour() >= hours[0] && sent.Hour() < hours[1] {
			return p.Multiplier
		}
	}
	return 1
}

// of is what this usage cost at this moment.
func (p Price) of(usage Usage, sent time.Time) float64 {
	cached := orElse(p.CachedInputPerMillion, p.InputPerMillion)
	written := orElse(p.CacheWritePerMillion, p.InputPerMillion)
	fresh := usage.Input - usage.Cached - usage.CacheWrite
	perMillion := float64(fresh)*p.InputPerMillion + float64(usage.Cached)*cached +
		float64(usage.CacheWrite)*written + float64(usage.Output)*p.OutputPerMillion
	return perMillion / 1e6 * p.Peak.at(sent)
}

func orElse(rate *float64, otherwise float64) float64 {
	if rate == nil {
		return otherwise
	}
	return *rate
}
