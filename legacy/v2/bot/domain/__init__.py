"""Domain objects shared by Telegram, Web API and background workers."""

from .actor import Actor
from .security import secret_fingerprint

__all__ = ["Actor", "secret_fingerprint"]
