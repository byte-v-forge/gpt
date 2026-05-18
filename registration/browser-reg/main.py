#!/usr/bin/env python3
"""
ChatGPT Browser Registration gRPC entry point.

Environment variables:
  GRPC_PORT         gRPC listen port (default: 50051)
"""

import argparse
import logging
import os

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
logger = logging.getLogger(__name__)


def run_serve(args):
    """gRPC service mode."""
    from browser_reg.server import serve

    grpc_port = args.grpc_port or int(os.environ.get("GRPC_PORT", "50051"))
    if args.debug:
        os.environ["BROWSER_REG_DEBUG"] = "1"

    logger.info(
        "[main] Starting gRPC service: grpc_port=%s workers=1 debug=%s",
        grpc_port,
        os.environ.get("BROWSER_REG_DEBUG", "").strip().lower() in ("1", "true", "yes", "on"),
    )
    serve(grpc_port=grpc_port)


def main():
    parser = argparse.ArgumentParser(description="ChatGPT Browser Registration")
    parser.add_argument("command", nargs="?", choices=["serve"], default="serve", help="Command to run")
    parser.add_argument("--grpc-port", type=int, default=0, help="gRPC listen port")
    parser.add_argument("--debug", action="store_true", help="Run browser flows in visible local debug mode")

    args = parser.parse_args()

    run_serve(args)


if __name__ == "__main__":
    main()
