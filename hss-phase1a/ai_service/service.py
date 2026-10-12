"""
Pulse AI Inference Service - YOLO11 Fire & Smoke Detector
Provides high-performance HTTP / REST endpoints for real-time fire and smoke detection.
"""

import os
import sys

# Silence OpenCV and FFmpeg internal stderr warning spam
os.environ["OPENCV_FFMPEG_LOGLEVEL"] = "-8"
os.environ["OPENCV_LOG_LEVEL"] = "SILENT"

import time
import io
import base64
import logging
import threading
import tempfile
import cv2
from typing import List, Optional, Dict, Any
from pathlib import Path

from fastapi import FastAPI, File, UploadFile, Query, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from PIL import Image
import numpy as np

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
logger = logging.getLogger("pulse_ai_service")

from contextlib import asynccontextmanager

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan handler to load and warm up model on startup."""
    try:
        load_model()
    except Exception as e:
        logger.warning(f"Startup model load deferred or failed: {e}")
    yield

# Initialize FastAPI app
app = FastAPI(
    title="Pulse YOLO11 Fire & Smoke AI Inference Service",
    description="Real-time object detection service for Fire and Smoke using Ultralytics YOLO11m",
    version="1.0.0",
    lifespan=lifespan,
)

# CORS middleware for direct web client telemetry if needed
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Global model state
_model_lock = threading.Lock()
_yolo_model = None
_model_path_used = None
_class_names = {0: "fire", 1: "smoke"}
_cached_sps_pps = b""

# Prometheus telemetry state
_metrics_lock = threading.Lock()
_ai_metrics = {
    "requests_total": 0,
    "fire_detections_total": 0,
    "smoke_detections_total": 0,
    "last_inference_ms": 0.0,
}

# Temporal Confirmation & Optical Flow state per camera
_temporal_lock = threading.Lock()
_camera_streaks: Dict[str, Dict[str, Any]] = {}
TEMPORAL_CONFIRM_FRAMES = int(os.environ.get("TEMPORAL_CONFIRM_FRAMES", "3"))

DEFAULT_CONFIDENCE_THRESHOLD = float(os.environ.get("CONFIDENCE_THRESHOLD", "0.35"))
DEFAULT_FIRE_THRESHOLD = float(os.environ.get("FIRE_THRESHOLD", "0.35"))
DEFAULT_SMOKE_THRESHOLD = float(os.environ.get("SMOKE_THRESHOLD", "0.30"))


def update_temporal_and_flow_state(
    camera_id: str,
    image: Image.Image,
    has_fire: bool,
    has_smoke: bool,
) -> Dict[str, Any]:
    """
    Tracks multi-frame temporal streaks (N consecutive frames) and dense Farneback
    optical flow motion energy per camera to verify sustained fire/smoke dynamics.
    """
    gray_small = np.array(image.resize((160, 120)).convert("L"), dtype=np.uint8)
    motion_score = 0.0

    with _temporal_lock:
        state = _camera_streaks.get(camera_id)
        if state is None:
            state = {
                "fire_streak": 0,
                "smoke_streak": 0,
                "prev_gray": None,
            }
            _camera_streaks[camera_id] = state

        prev_gray = state.get("prev_gray")
        if prev_gray is not None and prev_gray.shape == gray_small.shape:
            try:
                flow = cv2.calcOpticalFlowFarneback(
                    prev_gray, gray_small, None, 0.5, 2, 11, 2, 5, 1.1, 0
                )
                mag, _ = cv2.cartToPolar(flow[..., 0], flow[..., 1])
                motion_score = float(np.mean(mag))
            except Exception:
                motion_score = 0.0

        state["prev_gray"] = gray_small
        state["fire_streak"] = (state["fire_streak"] + 1) if has_fire else 0
        state["smoke_streak"] = (state["smoke_streak"] + 1) if has_smoke else 0

        fire_streak = state["fire_streak"]
        smoke_streak = state["smoke_streak"]
        confirmed = (
            fire_streak >= TEMPORAL_CONFIRM_FRAMES
            or smoke_streak >= TEMPORAL_CONFIRM_FRAMES
        )

    return {
        "fire_streak": fire_streak,
        "smoke_streak": smoke_streak,
        "temporal_confirmed": confirmed,
        "motion_score": round(motion_score, 4),
    }


def resolve_model_path() -> str:
    """Find the best.pt model file across standard project locations."""
    env_path = os.environ.get("MODEL_PATH")
    if env_path and os.path.exists(env_path):
        return os.path.abspath(env_path)

    current_dir = Path(__file__).resolve().parent
    candidate_paths = [
        current_dir.parent.parent / "best.pt",       # d:\projects\pulse-landing(3)\best.pt
        current_dir.parent / "best.pt",              # hss-phase1a/best.pt
        current_dir / "best.pt",                     # hss-phase1a/ai_service/best.pt
        Path("d:/projects/pulse-landing(3)/best.pt"),
        Path("best.pt"),
    ]

    for p in candidate_paths:
        if p.exists():
            return str(p.resolve())

    # Default fallback path
    return str(candidate_paths[0])


def load_model(model_path: Optional[str] = None):
    """Load and warmup YOLO11 model."""
    global _yolo_model, _model_path_used, _class_names
    with _model_lock:
        if _yolo_model is not None:
            return _yolo_model

        if not model_path:
            model_path = resolve_model_path()

        logger.info(f"Loading YOLO11 model from {model_path}...")
        try:
            from ultralytics import YOLO
            _yolo_model = YOLO(model_path)
            _model_path_used = model_path
            
            # Extract class names
            if hasattr(_yolo_model, "names") and isinstance(_yolo_model.names, dict):
                _class_names = _yolo_model.names
            elif hasattr(_yolo_model, "model") and hasattr(_yolo_model.model, "names"):
                _class_names = _yolo_model.model.names
            
            logger.info(f"Model loaded successfully. Classes: {_class_names}")

            # Pre-warmup inference with a blank frame
            warmup_img = np.zeros((448, 448, 3), dtype=np.uint8)
            _ = _yolo_model(warmup_img, conf=0.10, verbose=False)
            logger.info("Model warmup completed successfully.")
            return _yolo_model
        except Exception as e:
            logger.error(f"Failed to load YOLO model from {model_path}: {e}")
            raise


# --- Pydantic Data Models ---

class DetectionItem(BaseModel):
    class_id: int = Field(..., description="0 for fire, 1 for smoke")
    class_name: str = Field(..., description="'fire' or 'smoke'")
    confidence: float = Field(..., description="Detection confidence score (0.0 to 1.0)")
    box: List[float] = Field(..., description="Normalized bounding box [x, y, w, h] (top-left x, y, width, height, 0.0-1.0)")
    xyxy: Optional[List[float]] = Field(None, description="Absolute pixel coordinates [x1, y1, x2, y2]")
    xyxy_norm: Optional[List[float]] = Field(None, description="Normalized coordinates [x1, y1, x2, y2]")


class DetectionResponse(BaseModel):
    camera_id: str = Field("default", description="Camera identifier")
    timestamp: float = Field(..., description="Unix timestamp of detection")
    inference_time_ms: float = Field(..., description="Inference execution time in milliseconds")
    detections_count: int = Field(0, description="Total count of detections meeting threshold")
    has_fire: bool = Field(False, description="True if any fire detected with conf >= threshold")
    has_smoke: bool = Field(False, description="True if any smoke detected with conf >= threshold")
    fire_streak: int = Field(0, description="Consecutive frames with fire detected on this camera")
    smoke_streak: int = Field(0, description="Consecutive frames with smoke detected on this camera")
    temporal_confirmed: bool = Field(False, description="True if streak >= TEMPORAL_CONFIRM_FRAMES")
    motion_score: float = Field(0.0, description="Optical flow motion magnitude score")
    detections: List[DetectionItem] = Field(default_factory=list, description="List of detected objects")


class RawDetectionRequest(BaseModel):
    frame_base64: str = Field(..., description="Base64-encoded image frame (JPEG/PNG)")
    camera_id: Optional[str] = Field("default", description="Camera identifier")
    confidence_threshold: Optional[float] = Field(None, description="Confidence threshold (default: 0.35)")
    fire_threshold: Optional[float] = Field(None, description="Fire (class 0) confidence threshold")
    smoke_threshold: Optional[float] = Field(None, description="Smoke (class 1) confidence threshold")


# --- Inference Helper Logic ---

def run_detection_on_image(
    image: Image.Image,
    camera_id: str = "default",
    confidence_threshold: Optional[float] = None,
    fire_threshold: Optional[float] = None,
    smoke_threshold: Optional[float] = None,
) -> DetectionResponse:
    """Run YOLO inference on PIL Image and build DetectionResponse."""
    global _yolo_model
    if _yolo_model is None:
        load_model()

    start_time = time.perf_counter()
    img_width, img_height = image.size

    base_thresh = float(confidence_threshold) if confidence_threshold is not None else DEFAULT_CONFIDENCE_THRESHOLD
    eff_fire_thresh = fire_threshold if fire_threshold is not None else (
        base_thresh if confidence_threshold is not None else DEFAULT_FIRE_THRESHOLD
    )
    eff_smoke_thresh = smoke_threshold if smoke_threshold is not None else (
        base_thresh if confidence_threshold is not None else DEFAULT_SMOKE_THRESHOLD
    )

    infer_conf = max(0.01, min(base_thresh, eff_fire_thresh, eff_smoke_thresh))

    # Ultralytics inference
    with _model_lock:
        results = _yolo_model(image, conf=float(infer_conf), verbose=False)

    inference_time_ms = (time.perf_counter() - start_time) * 1000.0

    detections: List[DetectionItem] = []
    has_fire = False
    has_smoke = False

    if results and len(results) > 0:
        boxes = results[0].boxes
        if boxes is not None and len(boxes) > 0:
            for box in boxes:
                cls_id = int(box.cls[0].item())
                conf = float(box.conf[0].item())

                cls_name = _class_names.get(cls_id, "fire" if cls_id == 0 else "smoke")
                target_thresh = eff_fire_thresh if (cls_id == 0 or cls_name.lower() == "fire") else (
                    eff_smoke_thresh if (cls_id == 1 or cls_name.lower() == "smoke") else base_thresh
                )

                # Filter strictly by class-specific confidence threshold
                if conf < target_thresh:
                    continue

                if cls_id == 0 or cls_name.lower() == "fire":
                    has_fire = True
                elif cls_id == 1 or cls_name.lower() == "smoke":
                    has_smoke = True

                # Bounding box coordinates
                xyxy_abs = box.xyxy[0].tolist() # [x1, y1, x2, y2]
                x1, y1, x2, y2 = xyxy_abs

                # Normalized top-left x, y, width, height
                norm_x = max(0.0, min(1.0, float(x1 / img_width)))
                norm_y = max(0.0, min(1.0, float(y1 / img_height)))
                norm_w = max(0.0, min(1.0, float((x2 - x1) / img_width)))
                norm_h = max(0.0, min(1.0, float((y2 - y1) / img_height)))

                norm_x1 = max(0.0, min(1.0, float(x1 / img_width)))
                norm_y1 = max(0.0, min(1.0, float(y1 / img_height)))
                norm_x2 = max(0.0, min(1.0, float(x2 / img_width)))
                norm_y2 = max(0.0, min(1.0, float(y2 / img_height)))

                detections.append(
                    DetectionItem(
                        class_id=cls_id,
                        class_name=cls_name,
                        confidence=round(conf, 4),
                        box=[
                            round(norm_x, 4),
                            round(norm_y, 4),
                            round(norm_w, 4),
                            round(norm_h, 4),
                        ],
                        xyxy=[round(c, 2) for c in xyxy_abs],
                        xyxy_norm=[
                            round(norm_x1, 4),
                            round(norm_y1, 4),
                            round(norm_x2, 4),
                            round(norm_y2, 4),
                        ],
                    )
                )

    temporal_info = update_temporal_and_flow_state(
        camera_id=camera_id,
        image=image,
        has_fire=has_fire,
        has_smoke=has_smoke,
    )

    with _metrics_lock:
        _ai_metrics["requests_total"] += 1
        if has_fire:
            _ai_metrics["fire_detections_total"] += 1
        if has_smoke:
            _ai_metrics["smoke_detections_total"] += 1
        _ai_metrics["last_inference_ms"] = round(inference_time_ms, 2)

    return DetectionResponse(
        camera_id=camera_id,
        timestamp=time.time(),
        inference_time_ms=round(inference_time_ms, 2),
        detections_count=len(detections),
        has_fire=has_fire,
        has_smoke=has_smoke,
        fire_streak=temporal_info["fire_streak"],
        smoke_streak=temporal_info["smoke_streak"],
        temporal_confirmed=temporal_info["temporal_confirmed"],
        motion_score=temporal_info["motion_score"],
        detections=detections,
    )


def decode_image_or_h264(image_bytes: bytes) -> Image.Image:
    """
    Decodes raw payload into a PIL RGB Image.
    Supports JPEG/PNG/WebP image formats as well as Annex-B H.264 raw bitstreams.
    """
    if not image_bytes:
        raise HTTPException(status_code=400, detail="Empty image or video payload.")

    # Fast path: Try standard PIL decode
    try:
        return Image.open(io.BytesIO(image_bytes)).convert("RGB")
    except Exception:
        pass

    # Secondary fast path: OpenCV imdecode for JPEG / PNG bytes
    try:
        np_arr = np.frombuffer(image_bytes, np.uint8)
        img_cv = cv2.imdecode(np_arr, cv2.IMREAD_COLOR)
        if img_cv is not None and img_cv.size > 0:
            return Image.fromarray(cv2.cvtColor(img_cv, cv2.COLOR_BGR2RGB))
    except Exception:
        pass

    # Video path: If payload has H.264 Annex-B start codes, decode first frame using OpenCV
    if (
        image_bytes.startswith(b"\x00\x00\x00\x01")
        or image_bytes.startswith(b"\x00\x00\x01")
        or b"\x00\x00\x00\x01" in image_bytes[:32]
        or b"\x00\x00\x01" in image_bytes[:32]
    ):
        global _cached_sps_pps
        # Check if payload contains SPS (0x67)
        if b"\x00\x00\x00\x01\x67" in image_bytes or b"\x00\x00\x01\x67" in image_bytes:
            # Extract SPS+PPS prefix before IDR
            idx = image_bytes.find(b"\x00\x00\x00\x01\x65")
            if idx == -1:
                idx = image_bytes.find(b"\x00\x00\x01\x65")
            if idx > 0:
                _cached_sps_pps = image_bytes[:idx]
        elif _cached_sps_pps and not (b"\x67" in image_bytes[:8]):
            image_bytes = _cached_sps_pps + image_bytes

        tmp_path = None
        try:
            with tempfile.NamedTemporaryFile(suffix=".h264", delete=False) as tf:
                tf.write(image_bytes)
                tmp_path = tf.name

            cap = cv2.VideoCapture(tmp_path, cv2.CAP_FFMPEG)
            if cap.isOpened():
                for _ in range(5):
                    ret, frame = cap.read()
                    if ret and frame is not None and frame.size > 0:
                        cap.release()
                        return Image.fromarray(cv2.cvtColor(frame, cv2.COLOR_BGR2RGB))
                cap.release()
        except Exception as e:
            logger.warning(f"H.264 Annex-B decoding error: {e}")
        finally:
            if tmp_path and os.path.exists(tmp_path):
                try:
                    os.unlink(tmp_path)
                except OSError:
                    pass

    raise HTTPException(status_code=400, detail="Failed to decode image or H.264 video payload.")


# --- REST Endpoints ---

@app.get("/health")
@app.get("/")
async def health_check():
    """Health check endpoint exposing model state and configuration."""
    return {
        "status": "healthy",
        "service": "Pulse YOLO11 Fire & Smoke AI Inference Service",
        "model_loaded": _yolo_model is not None,
        "model_path": _model_path_used,
        "classes": _class_names,
        "default_threshold": DEFAULT_CONFIDENCE_THRESHOLD,
        "fire_threshold": DEFAULT_FIRE_THRESHOLD,
        "smoke_threshold": DEFAULT_SMOKE_THRESHOLD,
        "temporal_confirm_frames": TEMPORAL_CONFIRM_FRAMES,
    }


@app.get("/metrics")
async def prometheus_metrics():
    """Prometheus telemetry endpoint for AI inference statistics."""
    with _metrics_lock:
        reqs = _ai_metrics["requests_total"]
        fires = _ai_metrics["fire_detections_total"]
        smokes = _ai_metrics["smoke_detections_total"]
        lat = _ai_metrics["last_inference_ms"]
    body = (
        "# HELP pulse_ai_requests_total Total inference requests processed\n"
        "# TYPE pulse_ai_requests_total counter\n"
        f"pulse_ai_requests_total {reqs}\n\n"
        "# HELP pulse_ai_fire_detections_total Total frames with fire detected\n"
        "# TYPE pulse_ai_fire_detections_total counter\n"
        f"pulse_ai_fire_detections_total {fires}\n\n"
        "# HELP pulse_ai_smoke_detections_total Total frames with smoke detected\n"
        "# TYPE pulse_ai_smoke_detections_total counter\n"
        f"pulse_ai_smoke_detections_total {smokes}\n\n"
        "# HELP pulse_ai_last_inference_ms Latency of the most recent YOLO11 inference\n"
        "# TYPE pulse_ai_last_inference_ms gauge\n"
        f"pulse_ai_last_inference_ms {lat}\n"
    )
    return Response(content=body, media_type="text/plain; version=0.0.4")


@app.post("/detect", response_model=DetectionResponse)
async def detect(
    request: Request,
    file: Optional[UploadFile] = File(None),
    camera_id: str = Query("default", description="Camera identifier"),
    confidence_threshold: Optional[float] = Query(None, ge=0.01, le=1.0, description="Confidence threshold"),
    fire_threshold: Optional[float] = Query(None, ge=0.01, le=1.0, description="Fire (class 0) threshold"),
    smoke_threshold: Optional[float] = Query(None, ge=0.01, le=1.0, description="Smoke (class 1) threshold"),
):
    """
    Detect fire and smoke in an uploaded image frame.
    Accepts multipart/form-data image upload OR raw binary image body.
    """
    image_bytes = None

    if file is not None:
        image_bytes = await file.read()
    else:
        # Check if raw bytes sent directly in request body
        body = await request.body()
        if body and len(body) > 0:
            image_bytes = body

    if not image_bytes:
        raise HTTPException(
            status_code=400,
            detail="No image data provided. Send multipart 'file' or raw image bytes in request body.",
        )

    image = decode_image_or_h264(image_bytes)

    return run_detection_on_image(
        image=image,
        camera_id=camera_id,
        confidence_threshold=confidence_threshold,
        fire_threshold=fire_threshold,
        smoke_threshold=smoke_threshold,
    )


@app.post("/detect_raw", response_model=DetectionResponse)
async def detect_raw(payload: RawDetectionRequest):
    """
    Detect fire and smoke from a base64-encoded frame payload.
    """
    try:
        raw_b64 = payload.frame_base64
        if "," in raw_b64:
            raw_b64 = raw_b64.split(",", 1)[1]
        image_bytes = base64.b64decode(raw_b64)
        image = decode_image_or_h264(image_bytes)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Invalid base64 image data: {e}")

    return run_detection_on_image(
        image=image,
        camera_id=payload.camera_id or "default",
        confidence_threshold=payload.confidence_threshold,
        fire_threshold=payload.fire_threshold,
        smoke_threshold=payload.smoke_threshold,
    )


@app.post("/detect_path", response_model=DetectionResponse)
async def detect_path(
    path: str = Query(..., description="Absolute or relative file path on server"),
    camera_id: str = Query("default"),
    confidence_threshold: Optional[float] = Query(None),
    fire_threshold: Optional[float] = Query(None),
    smoke_threshold: Optional[float] = Query(None),
):
    """Run detection on a local file path (useful for testing and benchmarking)."""
    if not os.path.exists(path):
        raise HTTPException(status_code=404, detail=f"File not found: {path}")

    try:
        with open(path, "rb") as f:
            file_bytes = f.read()
        image = decode_image_or_h264(file_bytes)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Cannot open image file: {e}")

    return run_detection_on_image(
        image=image,
        camera_id=camera_id,
        confidence_threshold=confidence_threshold,
        fire_threshold=fire_threshold,
        smoke_threshold=smoke_threshold,
    )
