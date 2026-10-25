package shield

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var reasonClasses = map[string]string{
	"honeypot": "honeypot", "scanner-ua": "scanner_ua", "ai-crawler": "ai_crawler",
	"fake-search-bot": "fake_bot", "rate-limit": "rate", "banned": "banned",
	"unknown-host": "unknown_host", "sni-missing": "tls", "sni-ip": "tls",
	"sni-unknown": "tls", "auth-failure": "behavior", "probe-rate": "behavior",
	"enumeration": "behavior", "odd-request": "behavior", "empty-ua": "behavior",
	"fake-ua": "fake_bot", "browser-inconsistent": "fake_bot",
	"research-ua": "scanner_ua", "search-blocked": "other", "search-unverified": "other",
}

func reasonClass(r string) string {
	if c, ok := reasonClasses[r]; ok {
		return c
	}
	return "other"
}

// OTelSink reports shield decisions through an OpenTelemetry meter.
type OTelSink struct {
	ctx       context.Context
	decisions metric.Int64Counter
}

// NewOTelSink registers the counters and gauges on meter. shieldStats must
// return the current banned count and active tarpit count for the gauges.
func NewOTelSink(ctx context.Context, meter metric.Meter, shieldStats func() (int, int)) (*OTelSink, error) {
	s := &OTelSink{ctx: ctx}
	var err error
	s.decisions, err = meter.Int64Counter(
		"netbirdy_proxy_shield_decisions_total",
		metric.WithUnit("1"),
		metric.WithDescription("Shield decisions by action and reason class"),
	)
	if err != nil {
		return nil, err
	}
	_, err = meter.Int64ObservableGauge("netbirdy_proxy_shield_banned",
		metric.WithUnit("1"),
		metric.WithDescription("Currently banned source IPs"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			b, _ := shieldStats()
			o.Observe(int64(b))
			return nil
		}))
	if err != nil {
		return nil, err
	}
	_, err = meter.Int64ObservableGauge("netbirdy_proxy_shield_tarpits",
		metric.WithUnit("1"),
		metric.WithDescription("Currently active tarpits"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			_, t := shieldStats()
			o.Observe(int64(t))
			return nil
		}))
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *OTelSink) Decision(action, reason string) {
	s.decisions.Add(s.ctx, 1, metric.WithAttributes(
		attribute.String("action", action),
		attribute.String("reason_class", reasonClass(reason)),
	))
}
func (s *OTelSink) BannedGauge(n int)  {}
func (s *OTelSink) TarpitActive(n int) {}
