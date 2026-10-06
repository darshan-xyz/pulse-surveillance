package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
)

var (
	ActiveWebRTCSessions int64
	EventsEmitted        int64
	RecordingSegments    int64
)

// PrometheusHandler provides a lightweight metrics endpoint without bringing in large dependencies.
func PrometheusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Fprintf(w, "# HELP hss_active_webrtc_sessions Number of currently active WebRTC P2P streams\n")
	fmt.Fprintf(w, "# TYPE hss_active_webrtc_sessions gauge\n")
	fmt.Fprintf(w, "hss_active_webrtc_sessions %d\n\n", atomic.LoadInt64(&ActiveWebRTCSessions))

	fmt.Fprintf(w, "# HELP hss_events_emitted_total Total events emitted since boot\n")
	fmt.Fprintf(w, "# TYPE hss_events_emitted_total counter\n")
	fmt.Fprintf(w, "hss_events_emitted_total %d\n\n", atomic.LoadInt64(&EventsEmitted))

	fmt.Fprintf(w, "# HELP hss_recording_segments_total Total MP4 segments written\n")
	fmt.Fprintf(w, "# TYPE hss_recording_segments_total counter\n")
	fmt.Fprintf(w, "hss_recording_segments_total %d\n\n", atomic.LoadInt64(&RecordingSegments))

	fmt.Fprintf(w, "# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use\n")
	fmt.Fprintf(w, "# TYPE go_memstats_alloc_bytes gauge\n")
	fmt.Fprintf(w, "go_memstats_alloc_bytes %d\n", m.Alloc)
}
