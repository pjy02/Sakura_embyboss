from hashlib import sha256


def secret_fingerprint(value: str) -> str:
    """Stable non-reversible identifier suitable for logs and idempotency keys."""

    return sha256(value.encode("utf-8")).hexdigest()
