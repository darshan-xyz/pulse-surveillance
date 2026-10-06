package recording

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"hss/internal/config"
	"hss/internal/database"
	"hss/internal/media"
	"github.com/google/uuid"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/mediago/mp4"
)

type Recorder struct {
	CamID    string
	DataPath string
}

func NewRecorder(camID string) *Recorder {
	return &Recorder{
		CamID:    camID,
		DataPath: config.AppConfig.Storage.RecordingsPath,
	}
}

// Start initiates the segmenting recording pipeline.
func (r *Recorder) Start(ctx context.Context) {
	log.Printf("Starting recording pipeline for camera: %s\n", r.CamID)

	// Subscribe to raw H264 RTP packets from the Media Engine
	rtpChan, err := media.DefaultEngine.Subscribe(r.CamID)
	if err != nil {
		log.Printf("Recorder failed to subscribe to media engine: %v", err)
		return
	}

	segmentDuration := time.Duration(config.AppConfig.Camera.SegmentDurationSeconds) * time.Second
	ticker := time.NewTicker(segmentDuration)
	defer ticker.Stop()

	for {
		err := r.recordSegment(ctx, rtpChan, ticker.C)
		if err != nil {
			if err == context.Canceled {
				log.Printf("Shutting down recording pipeline for camera: %s\n", r.CamID)
				return
			}
			log.Printf("Segment error for camera %s: %v. Retrying in 5s...", r.CamID, err)
			time.Sleep(5 * time.Second)
		}
	}
}

func (r *Recorder) recordSegment(ctx context.Context, rtpChan <-chan *format.H264, tickChan <-chan time.Time) error {
	startedAt := time.Now()
	dir := filepath.Join(r.DataPath, r.CamID, startedAt.Format("2006"), startedAt.Format("01"), startedAt.Format("02"), startedAt.Format("15"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create recording directory: %v", err)
	}

	segmentID := "seg_" + uuid.New().String()[:8]
	tmpFile := filepath.Join(dir, segmentID+".tmp")
	finalFile := filepath.Join(dir, segmentID+".mp4")

	// 1. Open temporary file
	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp recording file: %v", err)
	}
	defer f.Close()

	// Initialize the H264 RTP Decoder
	decoder := &rtph264.Decoder{}
	decoder.Init()

	// Initialize the MP4 Muxer (via mediago)
	mux := mp4.NewMuxer(f)
	videoTrack := mp4.NewVideoTrackH264()
	if err := mux.AddTrack(videoTrack); err != nil {
		return fmt.Errorf("failed to add mp4 track: %v", err)
	}
	if err := mux.Start(); err != nil {
		return fmt.Errorf("failed to start mp4 muxer: %v", err)
	}

	log.Printf("Writing segment to %s...\n", tmpFile)

	var bytesWritten int64
	var firstPTS time.Duration
	var initializedPTS bool

writeLoop:
	for {
		select {
		case <-ctx.Done():
			break writeLoop
		case <-tickChan:
			// Time to cut a new segment
			break writeLoop
		case pkt := <-rtpChan:
			// Decode RTP into NALUs
			nalus, pts, err := decoder.Decode(pkt.RTP)
			if err != nil {
				continue // Skip malformed packets (e.g. fragmentation errors)
			}
			
			if !initializedPTS {
				firstPTS = pts
				initializedPTS = true
			}
			
			// Normalize PTS relative to start of segment
			samplePTS := pts - firstPTS

			// Write to MP4 Muxer
			err = videoTrack.WriteSample(nalus, samplePTS)
			if err != nil {
				log.Printf("Failed to write MP4 sample: %v", err)
			}
		}
	}

	// Finalize the MP4 file
	if err := mux.Close(); err != nil {
		log.Printf("Failed to close mp4 muxer cleanly: %v", err)
	}
	f.Close()

	// 3. Atomic rename
	if err := os.Rename(tmpFile, finalFile); err != nil {
		return fmt.Errorf("failed to finalize segment %s: %v", finalFile, err)
	}

	// Gather stats
	stat, _ := os.Stat(finalFile)
	if stat != nil {
		bytesWritten = stat.Size()
	}
	endedAt := time.Now()
	durationMs := endedAt.Sub(startedAt).Milliseconds()

	log.Printf("Finalized segment: %s (%d bytes)", finalFile, bytesWritten)

	// 4. Database Transaction
	query := `INSERT INTO recordings (id, camera_id, started_at, ended_at, path, size_bytes, duration_ms, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, dbErr := database.DB.Exec(query,
		segmentID, r.CamID,
		startedAt.Format(time.RFC3339),
		endedAt.Format(time.RFC3339),
		finalFile, bytesWritten, durationMs,
		time.Now().Format(time.RFC3339),
	)
	if dbErr != nil {
		log.Printf("Database error while saving recording metadata: %v", dbErr)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
