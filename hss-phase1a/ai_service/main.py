"""
Pulse AI Inference Service Entrypoint
Starts Uvicorn server hosting the YOLO11 Fire & Smoke detection service.
"""

import os
import sys
import argparse
import uvicorn

# Ensure ai_service and parent directories are in sys.path
current_dir = os.path.dirname(os.path.abspath(__file__))
if current_dir not in sys.path:
    sys.path.insert(0, current_dir)


def main():
    parser = argparse.ArgumentParser(description="Pulse YOLO11 Fire & Smoke Inference Service")
    parser.add_argument("--host", type=str, default="127.0.0.1", help="Host interface (default: 127.0.0.1)")
    parser.add_argument("--port", type=int, default=8000, help="Port to bind (default: 8000)")
    parser.add_argument("--model-path", type=str, default=None, help="Path to best.pt YOLO model")
    parser.add_argument("--workers", type=int, default=1, help="Number of worker processes")
    parser.add_argument("--log-level", type=str, default="info", help="Log level (debug, info, warning)")
    args = parser.parse_args()

    if args.model_path:
        os.environ["MODEL_PATH"] = os.path.abspath(args.model_path)

    print("=" * 60)
    print(" PULSE YOLO11 FIRE & SMOKE INFERENCE SERVICE")
    print("=" * 60)
    print(f" Binding to : {args.host}:{args.port}")
    print(f" Model Path : {os.environ.get('MODEL_PATH', 'auto-detect (best.pt)')}")
    print("=" * 60)

    uvicorn.run(
        "service:app",
        host=args.host,
        port=args.port,
        workers=args.workers,
        log_level=args.log_level.lower(),
        access_log=True,
    )


if __name__ == "__main__":
    main()
