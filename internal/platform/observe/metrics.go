// Levier — internal/platform/observe
//
// Dependency-free Prometheus metrics registry.
//
// Copyright 2026 The Levier Authors.
package observe

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DefaultHistogramBuckets covers tool and queue latencies from 5ms to 60s.
var DefaultHistogramBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60,
}

const (
	kindCounter   = "counter"
	kindGauge     = "gauge"
	kindHistogram = "histogram"
)

// Registry collects counters, gauges, and histograms and renders them as
// Prometheus text exposition.
type Registry struct {
	mu       sync.Mutex
	families map[string]*metricFamily
}

type metricFamily struct {
	name    string
	help    string
	kind    string
	series  map[string]*metricSeries
	buckets []float64
}

type metricSeries struct {
	labels  map[string]string
	value   float64
	count   uint64
	sum     float64
	buckets []uint64
}

// NewRegistry returns an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{families: make(map[string]*metricFamily)}
}

// Inc adds one to a counter.
func (r *Registry) Inc(name, help string, labels map[string]string) {
	r.Add(name, help, labels, 1)
}

// Add adds delta to a counter.
func (r *Registry) Add(name, help string, labels map[string]string, delta float64) {
	r.update(name, help, kindCounter, labels, func(series *metricSeries) {
		series.value += delta
	})
}

// SetCounter sets a cumulative counter to an absolute value.
func (r *Registry) SetCounter(name, help string, labels map[string]string, value float64) {
	r.update(name, help, kindCounter, labels, func(series *metricSeries) {
		series.value = value
	})
}

// Set stores a gauge value.
func (r *Registry) Set(name, help string, labels map[string]string, value float64) {
	r.update(name, help, kindGauge, labels, func(series *metricSeries) {
		series.value = value
	})
}

// AddGauge adds delta to a gauge.
func (r *Registry) AddGauge(name, help string, labels map[string]string, delta float64) {
	r.update(name, help, kindGauge, labels, func(series *metricSeries) {
		series.value += delta
	})
}

// Observe records one histogram observation.
func (r *Registry) Observe(name, help string, labels map[string]string, value float64) {
	r.update(name, help, kindHistogram, labels, func(series *metricSeries) {
		series.count++
		series.sum += value
		for index, upper := range DefaultHistogramBuckets {
			if value <= upper {
				series.buckets[index]++
			}
		}
	})
}

func (r *Registry) update(name, help, kind string, labels map[string]string, mutate func(*metricSeries)) {
	if r == nil || name == "" {
		return
	}
	key := labelKey(labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	family := r.families[name]
	if family == nil {
		family = &metricFamily{
			name:    name,
			help:    help,
			kind:    kind,
			series:  make(map[string]*metricSeries),
			buckets: DefaultHistogramBuckets,
		}
		r.families[name] = family
	}
	if family.kind != kind {
		return
	}
	series := family.series[key]
	if series == nil {
		series = &metricSeries{
			labels:  cloneLabels(labels),
			buckets: make([]uint64, len(DefaultHistogramBuckets)),
		}
		family.series[key] = series
	}
	mutate(series)
}

// WritePrometheus renders every family in deterministic order.
func (r *Registry) WritePrometheus(w io.Writer) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	names := make([]string, 0, len(r.families))
	for name := range r.families {
		names = append(names, name)
	}
	sort.Strings(names)
	snapshot := make([]*metricFamily, 0, len(names))
	for _, name := range names {
		family := r.families[name]
		copied := &metricFamily{
			name:    family.name,
			help:    family.help,
			kind:    family.kind,
			series:  make(map[string]*metricSeries, len(family.series)),
			buckets: append([]float64(nil), family.buckets...),
		}
		for key, series := range family.series {
			copied.series[key] = &metricSeries{
				labels:  cloneLabels(series.labels),
				value:   series.value,
				count:   series.count,
				sum:     series.sum,
				buckets: append([]uint64(nil), series.buckets...),
			}
		}
		snapshot = append(snapshot, copied)
	}
	r.mu.Unlock()

	for _, family := range snapshot {
		if family.help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", family.name, family.help); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", family.name, family.kind); err != nil {
			return err
		}
		keys := make([]string, 0, len(family.series))
		for key := range family.series {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			series := family.series[key]
			if family.kind == kindHistogram {
				if err := writeHistogram(w, family, series); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(w, "%s%s %s\n",
				family.name, formatLabels(series.labels), formatFloat(series.value)); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeHistogram(w io.Writer, family *metricFamily, series *metricSeries) error {
	for index, upper := range family.buckets {
		labels := cloneLabels(series.labels)
		labels["le"] = formatFloat(upper)
		if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n",
			family.name, formatLabels(labels), series.buckets[index]); err != nil {
			return err
		}
	}
	labels := cloneLabels(series.labels)
	labels["le"] = "+Inf"
	if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n",
		family.name, formatLabels(labels), series.count); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s_sum%s %s\n",
		family.name, formatLabels(series.labels), formatFloat(series.sum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s_count%s %d\n",
		family.name, formatLabels(series.labels), series.count); err != nil {
		return err
	}
	return nil
}

func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(labels[key])
		builder.WriteByte(0)
	}
	return builder.String()
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+`="`+escapeLabel(labels[key])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}
