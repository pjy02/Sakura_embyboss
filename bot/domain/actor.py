from dataclasses import dataclass
from typing import Optional


@dataclass(frozen=True)
class Actor:
    """Identity responsible for a business operation."""

    kind: str
    identifier: str
    display_name: Optional[str] = None

    @classmethod
    def telegram(cls, user_id: int, display_name: Optional[str] = None) -> "Actor":
        return cls(kind="telegram", identifier=str(user_id), display_name=display_name)

    @classmethod
    def web(cls, user_id: int, display_name: Optional[str] = None) -> "Actor":
        return cls(kind="web", identifier=str(user_id), display_name=display_name)

    @classmethod
    def system(cls, name: str) -> "Actor":
        return cls(kind="system", identifier=name, display_name=name)
