#!/usr/bin/env python3
"""Container liveness/readiness probe without third-party dependencies."""

import os
import sys
from urllib.error import HTTPError, URLError
from urllib.request import urlopen


def main() -> int:
    url = os.getenv("SAKURA_HEALTHCHECK_URL", "").strip()
    if url:
        try:
            with urlopen(url, timeout=3) as response:
                return 0 if 200 <= response.status < 300 else 1
        except (HTTPError, URLError, TimeoutError, OSError):
            return 1

    try:
        os.kill(int(os.getenv("SAKURA_HEALTHCHECK_PID", "1")), 0)
    except (OSError, ValueError):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
