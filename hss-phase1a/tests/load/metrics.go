package load

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LatencyTracker records duration samples and computes percentiles safely under concurrency.
type LatencyTracker struct {
	mu      sync.RWMutex
	samples []time.Duration
}

// NewLatencyTracker creates a tracker pre-allocated for expected capacity.
func NewLatencyTracker(capacity int) *LatencyTracker {
	if capacity <= 0 {
		capacity = 1024
	}
	return &LatencyTracker{
		samples: make([]time.Duration, 0, capacity),
	}
}

// Record appends a latency measurement.
func (lt *LatencyTracker) Record(d time.Duration) {
	lt.mu.Lock()
	lt.samples = append(lt.samples, d)
	lt.mu.Unlock()
}

// Stats computes summary statistics across all recorded samples.
type LatencyStats struct {
	Count  int
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	StdDev time.Duration
	P50    time.Duration
	P90    time.Duration
	P95    time.Duration
	P99    time.Duration
	P999   time.Duration
}

// Stats returns a calculated snapshot of latency statistics.
func (lt *LatencyTracker) Stats() LatencyStats {
	lt.mu.RLock()
	if len(lt.samples) == 0 {
		lt.mu.RUnlock()
		return LatencyStats{}
	}

	n := len(lt.samples)
	sorted := make([]time.Duration, n)
	copy(sorted, lt.samples)
	lt.mu.RUnlock()

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	var total time.Duration
	for _, d := range sorted {
		total += d
	}
	mean := total / time.Duration(n)

	var varianceSum float64
	meanNs := float64(mean.Nanoseconds())
	for _, d := range sorted {
		diff := float64(d.Nanoseconds()) - meanNs
		varianceSum += diff * diff
	}
	stdDev := time.Duration(math.Sqrt(varianceSum / float64(n)))

	percentile := func(p float64) time.Duration {
		if n == 0 {
			return 0
		}
		idx := int(math.Ceil((p/100.0)*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return sorted[idx]
	}

	return LatencyStats{
		Count:  n,
		Min:    sorted[0],
		Max:    sorted[n-1],
		Mean:   mean,
		StdDev: stdDev,
		P50:    percentile(50),
		P90:    percentile(90),
		P95:    percentile(95),
		P99:    percentile(99),
		P999:   percentile(99.9),
	}
}

// LoadMetrics tracks end-to-end load test metrics, throughput, errors, and latencies.
type LoadMetrics struct {
	mu                   sync.RWMutex
	TotalConnections     int64
	SuccessfulHandshakes int64
	FailedHandshakes     int64
	MessagesSent         int64
	MessagesReceived     int64
	ErrorsByType         map[string]int64
	HandshakeLatencies   *LatencyTracker
	ConnectLatencies     *LatencyTracker
	StartTime            time.Time
	EndTime              time.Time
}

// NewLoadMetrics creates an initialized load metrics collector.
func NewLoadMetrics(expectedConnections int) *LoadMetrics {
	return &LoadMetrics{
		ErrorsByType:       make(map[string]int64),
		HandshakeLatencies: NewLatencyTracker(expectedConnections),
		ConnectLatencies:   NewLatencyTracker(expectedConnections),
		StartTime:          time.Now(),
	}
}

// RecordConnect records a connection attempt.
func (m *LoadMetrics) RecordConnect(d time.Duration, success bool, err error) {
	atomic.AddInt64(&m.TotalConnections, 1)
	m.ConnectLatencies.Record(d)
	if !success && err != nil {
		m.RecordError(err)
	}
}

// RecordHandshake records the complete SDP offer/answer/candidate negotiation outcome.
func (m *LoadMetrics) RecordHandshake(d time.Duration, success bool, err error) {
	if success {
		atomic.AddInt64(&m.SuccessfulHandshakes, 1)
		m.HandshakeLatencies.Record(d)
	} else {
		atomic.AddInt64(&m.FailedHandshakes, 1)
		if err != nil {
			m.RecordError(err)
		}
	}
}

// RecordMsgSent tracks a sent WebSocket message.
func (m *LoadMetrics) RecordMsgSent() {
	atomic.AddInt64(&m.MessagesSent, 1)
}

// RecordMsgReceived tracks a received WebSocket message.
func (m *LoadMetrics) RecordMsgReceived() {
	atomic.AddInt64(&m.MessagesReceived, 1)
}

// RecordError categorizes and records an error occurrence.
func (m *LoadMetrics) RecordError(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	errMsg := err.Error()
	// Normalize common error prefixes
	switch {
	case strings.Contains(errMsg, "handshake"):
		m.ErrorsByType["websocket_handshake_error"]++
	case strings.Contains(errMsg, "timeout"):
		m.ErrorsByType["timeout_error"]++
	case strings.Contains(errMsg, "429"):
		m.ErrorsByType["rate_limit_429"]++
	case strings.Contains(errMsg, "401") || strings.Contains(errMsg, "403"):
		m.ErrorsByType["auth_error"]++
	case strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "connectex"):
		m.ErrorsByType["conn_refused"]++
	default:
		m.ErrorsByType[errMsg]++
	}
}

// Close marks the end time for metrics calculation.
func (m *LoadMetrics) Close() {
	m.EndTime = time.Now()
}

// Duration returns elapsed test duration.
func (m *LoadMetrics) Duration() time.Duration {
	if m.EndTime.IsZero() {
		return time.Since(m.StartTime)
	}
	return m.EndTime.Sub(m.StartTime)
}

// SuccessRate returns percentage of successful handshakes (0.0 - 100.0).
func (m *LoadMetrics) SuccessRate() float64 {
	total := atomic.LoadInt64(&m.SuccessfulHandshakes) + atomic.LoadInt64(&m.FailedHandshakes)
	if total == 0 {
		return 0.0
	}
	return (float64(atomic.LoadInt64(&m.SuccessfulHandshakes)) / float64(total)) * 100.0
}

// HandshakeRate returns handshakes per second.
func (m *LoadMetrics) HandshakeRate() float64 {
	sec := m.Duration().Seconds()
	if sec <= 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.SuccessfulHandshakes)) / sec
}

// MessageThroughput returns total messages processed per second.
func (m *LoadMetrics) MessageThroughput() float64 {
	sec := m.Duration().Seconds()
	if sec <= 0 {
		return 0
	}
	totalMsgs := atomic.LoadInt64(&m.MessagesSent) + atomic.LoadInt64(&m.MessagesReceived)
	return float64(totalMsgs) / sec
}

// Summary generates a formatted human-readable report.
func (m *LoadMetrics) Summary() string {
	hStats := m.HandshakeLatencies.Stats()
	cStats := m.ConnectLatencies.Stats()

	var sb strings.Builder
	sb.WriteString("\n================ LOAD TEST EXECUTION SUMMARY ================\n")
	sb.WriteString(fmt.Sprintf("Duration:             %v\n", m.Duration().Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("Total Connections:    %d\n", atomic.LoadInt64(&m.TotalConnections)))
	sb.WriteString(fmt.Sprintf("Successful Handshakes:%d\n", atomic.LoadInt64(&m.SuccessfulHandshakes)))
	sb.WriteString(fmt.Sprintf("Failed Handshakes:    %d\n", atomic.LoadInt64(&m.FailedHandshakes)))
	sb.WriteString(fmt.Sprintf("Success Rate:         %.2f%%\n", m.SuccessRate()))
	sb.WriteString(fmt.Sprintf("Handshake Throughput: %.2f handshakes/sec\n", m.HandshakeRate()))
	sb.WriteString(fmt.Sprintf("Messages Sent:        %d\n", atomic.LoadInt64(&m.MessagesSent)))
	sb.WriteString(fmt.Sprintf("Messages Received:    %d\n", atomic.LoadInt64(&m.MessagesReceived)))
	sb.WriteString(fmt.Sprintf("Message Throughput:   %.2f msgs/sec\n", m.MessageThroughput()))
	sb.WriteString("----------------- Latency Percentiles (Handshake) -------------\n")
	sb.WriteString(fmt.Sprintf("  Min:   %v\n", hStats.Min.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  Mean:  %v (stddev: %v)\n", hStats.Mean.Round(time.Microsecond), hStats.StdDev.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  P50:   %v\n", hStats.P50.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  P90:   %v\n", hStats.P90.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  P95:   %v\n", hStats.P95.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  P99:   %v\n", hStats.P99.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  P99.9: %v\n", hStats.P999.Round(time.Microsecond)))
	sb.WriteString(fmt.Sprintf("  Max:   %v\n", hStats.Max.Round(time.Microsecond)))
	sb.WriteString("----------------- Latency Percentiles (Connect) ---------------\n")
	sb.WriteString(fmt.Sprintf("  P50:   %v | P95: %v | P99: %v\n", cStats.P50.Round(time.Microsecond), cStats.P95.Round(time.Microsecond), cStats.P99.Round(time.Microsecond)))

	m.mu.RLock()
	if len(m.ErrorsByType) > 0 {
		sb.WriteString("----------------- Error Breakdown -----------------------------\n")
		for errType, count := range m.ErrorsByType {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", errType, count))
		}
	}
	m.mu.RUnlock()
	sb.WriteString("==============================================================\n")
	return sb.String()
}

// MemorySnapshot captures point-in-time runtime memory and goroutine stats.
type MemorySnapshot struct {
	AllocBytes      uint64
	TotalAllocBytes uint64
	SysBytes        uint64
	HeapAllocBytes  uint64
	HeapInuseBytes  uint64
	HeapIdleBytes   uint64
	HeapReleasedBytes uint64
	NumGC           uint32
	NumGoroutines   int
	Timestamp       time.Time
}

// CaptureMemorySnapshot takes an immediate snapshot of current Go runtime memory.
func CaptureMemorySnapshot() MemorySnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return MemorySnapshot{
		AllocBytes:        m.Alloc,
		TotalAllocBytes:   m.TotalAlloc,
		SysBytes:          m.Sys,
		HeapAllocBytes:    m.HeapAlloc,
		HeapInuseBytes:    m.HeapInuse,
		HeapIdleBytes:     m.HeapIdle,
		HeapReleasedBytes: m.HeapReleased,
		NumGC:             m.NumGC,
		NumGoroutines:     runtime.NumGoroutine(),
		Timestamp:         time.Now(),
	}
}

// MemoryDelta calculates differences between two snapshots.
type MemoryDelta struct {
	HeapAllocDeltaMB float64
	SysDeltaMB       float64
	GoroutineDelta   int
	NumGCDelta       uint32
	Duration         time.Duration
}

// CalculateMemoryDelta computes the delta between a baseline and post snapshot.
func CalculateMemoryDelta(baseline, current MemorySnapshot) MemoryDelta {
	heapDelta := float64(int64(current.HeapAllocBytes)-int64(baseline.HeapAllocBytes)) / (1024 * 1024)
	sysDelta := float64(int64(current.SysBytes)-int64(baseline.SysBytes)) / (1024 * 1024)
	return MemoryDelta{
		HeapAllocDeltaMB: heapDelta,
		SysDeltaMB:       sysDelta,
		GoroutineDelta:   current.NumGoroutines - baseline.NumGoroutines,
		NumGCDelta:       current.NumGC - baseline.NumGC,
		Duration:         current.Timestamp.Sub(baseline.Timestamp),
	}
}

// Summary outputs delta details.
func (d MemoryDelta) Summary() string {
	return fmt.Sprintf("Heap Delta: %+.2f MB | Sys Delta: %+.2f MB | Goroutine Delta: %+d | GC Cycles: %d",
		d.HeapAllocDeltaMB, d.SysDeltaMB, d.GoroutineDelta, d.NumGCDelta)
}
