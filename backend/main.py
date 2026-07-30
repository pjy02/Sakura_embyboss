import os

import uvicorn

from backend.settings import get_settings


def main():
    settings = get_settings()
    uvicorn.run(
        "backend.app:app",
        host=settings.host,
        port=settings.port,
        proxy_headers=True,
        forwarded_allow_ips=os.getenv(
            "SAKURA_FORWARDED_ALLOW_IPS",
            "127.0.0.1",
        ),
        log_level="info",
    )


if __name__ == "__main__":
    main()
