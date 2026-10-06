"""
Empirical Challenger Test Suite for YOLO11 Fire & Smoke Inference Service
Performs rigorous edge-case mining, boundary condition checks, coordinate validation,
and stress testing on best.pt and service.py.
"""

import os
import sys
import io
import time
import base64
import random
import threading
import concurrent.futures
from pathlib import Path
from typing import List, Dict, Any

from PIL import Image, ImageDraw
import numpy as np
import pytest
from fastapi.testclient import TestClient

# Ensure ai_service module is accessible
current_dir = Path(__file__).resolve().parent
sys.path.insert(0, str(current_dir))

from service import app, load_model, resolve_model_path, run_detection_on_image, _class_names

client = TestClient(app)


@pytest.fixture(scope="session", autouse=True)
def init_model():
    """Ensure model is loaded."""
    model = load_model()
    assert model is not None, "Model failed to load"
    return model


class TestEmpiricalModelIntegrity:
    """1. Model weight inspection and class contract verification."""

    def test_model_path_and_classes(self):
        model_path = resolve_model_path()
        assert os.path.exists(model_path), f"Model file not found at: {model_path}"
        assert os.path.getsize(model_path) > 1_000_000, "best.pt is unexpectedly small"
        
        # Verify class definitions
        assert 0 in _class_names or "0" in _class_names, "Class 0 missing in model names"
        assert 1 in _class_names or "1" in _class_names, "Class 1 missing in model names"
        
        name_0 = str(_class_names.get(0, _class_names.get("0"))).lower()
        name_1 = str(_class_names.get(1, _class_names.get("1"))).lower()
        assert name_0 == "fire", f"Expected class 0 to be 'fire', got: {name_0}"
        assert name_1 == "smoke", f"Expected class 1 to be 'smoke', got: {name_1}"


class TestEmpiricalDetectionOnRealFrames:
    """2. Test real and synthetic fire/smoke test frames."""

    def test_sample_fire_frame_detection(self):
        sample_path = current_dir / "test_data" / "frame_04_fire_smoke.jpg"
        assert sample_path.exists(), f"Sample test frame not found: {sample_path}"
        
        img = Image.open(sample_path).convert("RGB")
        resp = run_detection_on_image(img, camera_id="test-cam-04", confidence_threshold=0.40)
        
        assert resp.has_fire is True, "Fire frame failed to trigger has_fire=True"
        assert resp.detections_count >= 1, "Expected at least 1 detection"
        assert len(resp.detections) >= 1
        
        # Verify primary detection properties
        fire_det = [d for d in resp.detections if d.class_id == 0 or d.class_name == "fire"][0]
        assert fire_det.confidence >= 0.40, f"Confidence {fire_det.confidence} < 0.40 threshold"
        assert fire_det.class_name == "fire"
        assert fire_det.class_id == 0

    def test_synthetic_smoke_frame(self):
        # Create synthetic smoky scene
        img = Image.new('RGB', (640, 480), color=(100, 100, 100))
        draw = ImageDraw.Draw(img)
        # Smoky cloud gradient
        for r in range(120, 20, -15):
            alpha_gray = 130 + r
            draw.ellipse([200 - r, 150 - r, 200 + r, 150 + r], fill=(alpha_gray, alpha_gray, alpha_gray))
            draw.ellipse([350 - r, 200 - r, 350 + r, 200 + r], fill=(alpha_gray - 20, alpha_gray - 20, alpha_gray - 20))
        
        # Test inference - should execute without error
        resp = run_detection_on_image(img, confidence_threshold=0.10)
        assert isinstance(resp.detections, list)
        assert resp.inference_time_ms > 0


class TestBoundaryAndThresholdConditions:
    """3. Verify strict boundary thresholding and filtering."""

    def test_threshold_cutoff_exactness(self):
        sample_path = current_dir / "test_data" / "frame_04_fire_smoke.jpg"
        img = Image.open(sample_path).convert("RGB")

        # Run with very low threshold to discover actual confidence
        resp_low = run_detection_on_image(img, confidence_threshold=0.05)
        assert len(resp_low.detections) > 0
        actual_conf = resp_low.detections[0].confidence

        # Run with threshold just above actual confidence -> should filter out
        resp_above = run_detection_on_image(img, confidence_threshold=round(actual_conf + 0.05, 2))
        matching = [d for d in resp_above.detections if d.confidence < round(actual_conf + 0.05, 2)]
        assert len(matching) == 0, "Detections below threshold were not filtered"

        # Run with threshold just below actual confidence -> should include
        if actual_conf > 0.10:
            resp_below = run_detection_on_image(img, confidence_threshold=round(actual_conf - 0.05, 2))
            assert any(d.confidence >= round(actual_conf - 0.05, 2) for d in resp_below.detections)

    def test_threshold_extremes(self):
        img = Image.new('RGB', (320, 240), color=(50, 50, 50))
        
        # Threshold 1.0 (Maximum)
        resp_max = run_detection_on_image(img, confidence_threshold=1.0)
        assert resp_max.detections_count == 0
        assert resp_max.has_fire is False
        assert resp_max.has_smoke is False

        # Threshold 0.01 (Minimum)
        resp_min = run_detection_on_image(img, confidence_threshold=0.01)
        assert isinstance(resp_min.detections, list)


class TestNormalizedCoordinatesContract:
    """4. Verify bounding box normalization [0.0 .. 1.0] contract."""

    def test_coordinate_ranges(self):
        sample_path = current_dir / "test_data" / "frame_04_fire_smoke.jpg"
        img = Image.open(sample_path).convert("RGB")
        resp = run_detection_on_image(img, confidence_threshold=0.20)

        for i, det in enumerate(resp.detections):
            # Check normalized [x, y, w, h]
            assert len(det.box) == 4, f"Box {i} must have 4 elements"
            x, y, w, h = det.box
            assert 0.0 <= x <= 1.0, f"Box x ({x}) out of [0.0, 1.0]"
            assert 0.0 <= y <= 1.0, f"Box y ({y}) out of [0.0, 1.0]"
            assert 0.0 <= w <= 1.0, f"Box w ({w}) out of [0.0, 1.0]"
            assert 0.0 <= h <= 1.0, f"Box h ({h}) out of [0.0, 1.0]"
            assert x + w <= 1.05, f"Box right edge (x+w={x+w}) exceeds normalized boundary"
            assert y + h <= 1.05, f"Box bottom edge (y+h={y+h}) exceeds normalized boundary"

            # Check normalized [x1, y1, x2, y2] if present
            if det.xyxy_norm:
                nx1, ny1, nx2, ny2 = det.xyxy_norm
                assert 0.0 <= nx1 <= 1.0
                assert 0.0 <= ny1 <= 1.0
                assert 0.0 <= nx2 <= 1.0
                assert 0.0 <= ny2 <= 1.0
                assert nx1 <= nx2, f"x1 ({nx1}) > x2 ({nx2})"
                assert ny1 <= ny2, f"y1 ({ny1}) > y2 ({ny2})"

            # Check absolute pixel coordinates
            if det.xyxy:
                ax1, ay1, ax2, ay2 = det.xyxy
                assert 0.0 <= ax1 <= img.width + 1
                assert 0.0 <= ay1 <= img.height + 1
                assert ax1 <= ax2
                assert ay1 <= ay2


class TestAdversarialEdgeCases:
    """5. Adversarial stress tests: blank, noise, extreme sizes, malformed data."""

    def test_all_black_frame(self):
        img = Image.new('RGB', (640, 480), color=(0, 0, 0))
        resp = run_detection_on_image(img, confidence_threshold=0.40)
        assert resp.has_fire is False
        assert resp.has_smoke is False
        assert resp.detections_count == 0

    def test_all_white_frame(self):
        img = Image.new('RGB', (640, 480), color=(255, 255, 255))
        resp = run_detection_on_image(img, confidence_threshold=0.40)
        assert resp.has_fire is False
        assert resp.has_smoke is False
        assert resp.detections_count == 0

    def test_random_noise_frame(self):
        # Uniform noise
        noise_arr = np.random.randint(0, 256, (480, 640, 3), dtype=np.uint8)
        img = Image.fromarray(noise_arr)
        resp = run_detection_on_image(img, confidence_threshold=0.40)
        assert resp.inference_time_ms > 0

    def test_tiny_resolution(self):
        # 16x16 frame
        img = Image.new('RGB', (16, 16), color=(200, 50, 0))
        resp = run_detection_on_image(img, confidence_threshold=0.40)
        assert isinstance(resp.detections, list)

    def test_high_resolution_4k(self):
        # 4K UHD frame (3840x2160)
        img = Image.new('RGB', (3840, 2160), color=(30, 30, 30))
        draw = ImageDraw.Draw(img)
        draw.polygon([(1800, 1500), (1920, 900), (2040, 1500)], fill=(255, 80, 0))
        resp = run_detection_on_image(img, confidence_threshold=0.40)
        assert resp.inference_time_ms > 0
        for d in resp.detections:
            assert 0.0 <= d.box[0] <= 1.0
            assert 0.0 <= d.box[1] <= 1.0

    def test_extreme_aspect_ratios(self):
        # Ultra wide (2000 x 50)
        img_wide = Image.new('RGB', (2000, 50), color=(40, 40, 40))
        resp_wide = run_detection_on_image(img_wide, confidence_threshold=0.40)
        assert resp_wide.inference_time_ms > 0

        # Ultra tall (50 x 2000)
        img_tall = Image.new('RGB', (50, 2000), color=(40, 40, 40))
        resp_tall = run_detection_on_image(img_tall, confidence_threshold=0.40)
        assert resp_tall.inference_time_ms > 0

    def test_malformed_empty_payload(self):
        # Empty POST body to /detect
        response = client.post("/detect", content=b"", headers={"Content-Type": "image/jpeg"})
        assert response.status_code == 400
        assert "No image data provided" in response.json()["detail"]

    def test_corrupted_jpeg_bytes(self):
        # Corrupted image bytes
        corrupted_bytes = b"\xFF\xD8\xFF\xE0" + os.urandom(64) + b"\x00\x00"
        response = client.post("/detect", content=corrupted_bytes, headers={"Content-Type": "image/jpeg"})
        assert response.status_code == 400
        assert "Failed to decode image" in response.json()["detail"]

    def test_non_image_garbage(self):
        # Plain text
        response = client.post("/detect", content=b"THIS IS NOT AN IMAGE", headers={"Content-Type": "text/plain"})
        assert response.status_code == 400

    def test_malformed_base64_payload(self):
        # Invalid base64 characters
        payload = {"frame_base64": "!!!NOT_VALID_BASE64@@@###"}
        response = client.post("/detect_raw", json=payload)
        assert response.status_code == 400
        assert "Invalid base64 image data" in response.json()["detail"]

    def test_detect_path_missing_file(self):
        response = client.post("/detect_path?path=d:/non_existent_file_path_12345.jpg")
        assert response.status_code == 404
        assert "File not found" in response.json()["detail"]


class TestPerformanceAndConcurrency:
    """6. Performance latency and concurrent execution stress."""

    def test_sequential_latency_benchmark(self):
        sample_path = current_dir / "test_data" / "frame_04_fire_smoke.jpg"
        img = Image.open(sample_path).convert("RGB")
        
        latencies = []
        for _ in range(5):
            t0 = time.perf_counter()
            resp = run_detection_on_image(img, confidence_threshold=0.40)
            lat = (time.perf_counter() - t0) * 1000.0
            latencies.append(lat)
            assert resp.has_fire is True

        mean_lat = sum(latencies) / len(latencies)
        min_lat = min(latencies)
        max_lat = max(latencies)
        print(f"\n[PERF] Sequential Latency: mean={mean_lat:.1f}ms, min={min_lat:.1f}ms, max={max_lat:.1f}ms")
        assert mean_lat < 3000.0, f"Average inference too slow: {mean_lat}ms"

    def test_concurrent_burst_safety(self):
        sample_path = current_dir / "test_data" / "frame_04_fire_smoke.jpg"
        img = Image.open(sample_path).convert("RGB")

        num_workers = 6
        results = []
        errors = []

        def worker_fn(idx):
            try:
                resp = run_detection_on_image(img, camera_id=f"burst-cam-{idx}", confidence_threshold=0.40)
                return resp
            except Exception as e:
                return e

        with concurrent.futures.ThreadPoolExecutor(max_workers=num_workers) as executor:
            futures = [executor.submit(worker_fn, i) for i in range(num_workers)]
            for fut in concurrent.futures.as_completed(futures):
                res = fut.result()
                if isinstance(res, Exception):
                    errors.append(res)
                else:
                    results.append(res)

        assert len(errors) == 0, f"Concurrent execution errors: {errors}"
        assert len(results) == num_workers, f"Expected {num_workers} results, got {len(results)}"
        for res in results:
            assert res.has_fire is True
            assert res.detections_count >= 1


if __name__ == "__main__":
    pytest.main(["-v", "-s", __file__])
