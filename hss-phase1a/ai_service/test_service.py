"""
Test Suite for Pulse AI Inference Service
Verifies FastAPI endpoints, YOLO11 inference, confidence thresholding, and output schema.
"""

import os
import sys
import io
import time
from pathlib import Path
from PIL import Image, ImageDraw
import pytest
from fastapi.testclient import TestClient

# Ensure ai_service is on path
current_dir = Path(__file__).resolve().parent
sys.path.insert(0, str(current_dir))

from service import app, load_model, resolve_model_path, run_detection_on_image

client = TestClient(app)


@pytest.fixture(scope="session", autouse=True)
def setup_model():
    """Ensure YOLO model is loaded before running tests."""
    load_model()


def get_fire_test_frame() -> Image.Image:
    """Create or load image with simulated fire flame (verified to trigger YOLO11 detector with conf >= 0.40)."""
    # Check if frame_04_fire_smoke.jpg exists in survey directory or local test_data
    sample_paths = [
        current_dir / "test_data" / "frame_04_fire_smoke.jpg",
        current_dir.parent.parent / ".agents" / "teamwork_preview_explorer_survey_3" / "test_samples" / "frame_04_fire_smoke.jpg",
    ]
    for sp in sample_paths:
        if sp.exists():
            return Image.open(sp).convert("RGB")

    # Recreate frame_04 exact shapes
    img = Image.new('RGB', (640, 480), color=(40, 40, 45))
    draw = ImageDraw.Draw(img)
    draw.ellipse([240, 60, 420, 220], fill=(150, 150, 155))
    draw.ellipse([220, 120, 440, 280], fill=(130, 130, 135))
    draw.polygon([(270, 420), (330, 240), (390, 420)], fill=(255, 50, 0))
    draw.polygon([(290, 420), (330, 270), (370, 420)], fill=(255, 180, 0))
    draw.polygon([(305, 420), (330, 300), (355, 420)], fill=(255, 255, 220))
    return img


def get_normal_frame() -> Image.Image:
    """Create normal room image without fire or smoke."""
    img = Image.new('RGB', (640, 480), color=(120, 130, 140))
    draw = ImageDraw.Draw(img)
    draw.rectangle([50, 50, 200, 400], fill=(80, 90, 100))
    draw.rectangle([300, 100, 550, 300], fill=(60, 70, 80))
    return img


def test_health_endpoint():
    """Verify /health returns 200 and valid schema."""
    response = client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["model_loaded"] is True
    assert "fire" in str(data["classes"])
    assert "smoke" in str(data["classes"])


def test_detect_normal_frame():
    """Verify detection on a clean frame produces zero detections meeting threshold."""
    img = get_normal_frame()
    buf = io.BytesIO()
    img.save(buf, format="JPEG")
    buf.seek(0)

    response = client.post(
        "/detect?camera_id=test-cam-normal&confidence_threshold=0.40",
        files={"file": ("frame.jpg", buf, "image/jpeg")},
    )
    assert response.status_code == 200
    data = response.json()
    assert data["camera_id"] == "test-cam-normal"
    assert data["has_fire"] is False
    assert data["has_smoke"] is False
    assert len(data["detections"]) == 0


def test_detect_fire_frame_raw_body():
    """Verify detection on a fire frame using raw binary body."""
    img = get_fire_test_frame()
    buf = io.BytesIO()
    img.save(buf, format="JPEG")
    buf.seek(0)

    response = client.post(
        "/detect?camera_id=test-cam-fire&confidence_threshold=0.40",
        content=buf.getvalue(),
        headers={"Content-Type": "image/jpeg"},
    )
    assert response.status_code == 200
    data = response.json()
    assert data["camera_id"] == "test-cam-fire"
    assert "inference_time_ms" in data
    assert data["inference_time_ms"] > 0
    # Model should detect fire with >= 0.40 confidence
    assert data["has_fire"] is True
    assert len(data["detections"]) >= 1

    det = data["detections"][0]
    assert det["class_name"] == "fire"
    assert det["class_id"] == 0
    assert det["confidence"] >= 0.40
    # Validate normalized box format [x, y, w, h]
    assert len(det["box"]) == 4
    x, y, w, h = det["box"]
    assert 0.0 <= x <= 1.0
    assert 0.0 <= y <= 1.0
    assert 0.0 < w <= 1.0
    assert 0.0 < h <= 1.0


def test_detect_raw_base64():
    """Verify base64 detection endpoint."""
    import base64
    img = get_fire_test_frame()
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    b64_str = base64.b64encode(buf.getvalue()).decode("utf-8")

    payload = {
        "frame_base64": b64_str,
        "camera_id": "b64-cam",
        "confidence_threshold": 0.40,
    }
    response = client.post("/detect_raw", json=payload)
    assert response.status_code == 200
    data = response.json()
    assert data["camera_id"] == "b64-cam"
    assert data["has_fire"] is True
    assert len(data["detections"]) >= 1


def test_confidence_threshold_filtering():
    """Verify threshold filtering strictly excludes low confidence detections."""
    img = get_fire_test_frame()
    
    # Very high threshold (e.g. 0.999) should yield 0 detections
    res_high = run_detection_on_image(img, confidence_threshold=0.999)
    assert res_high.detections_count == 0
    assert res_high.has_fire is False

    # Standard threshold 0.40 should detect fire
    res_std = run_detection_on_image(img, confidence_threshold=0.40)
    assert res_std.detections_count >= 1
    assert res_std.has_fire is True


if __name__ == "__main__":
    print("Running AI Service Unit Tests...")
    import pytest
    sys.exit(pytest.main(["-v", __file__]))
