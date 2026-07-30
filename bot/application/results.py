from dataclasses import asdict, dataclass, field
from datetime import datetime
from typing import Any, Dict, Optional


@dataclass
class ServiceResult:
    status: str
    data: Dict[str, Any] = field(default_factory=dict)
    replayed: bool = False

    @property
    def ok(self) -> bool:
        return self.status == "ok"

    def to_dict(self) -> dict:
        value = asdict(self)
        for key, item in value["data"].items():
            if isinstance(item, datetime):
                value["data"][key] = item.isoformat()
        return value

    @classmethod
    def from_dict(cls, value: dict) -> "ServiceResult":
        return cls(
            status=value["status"],
            data=value.get("data") or {},
            replayed=True,
        )


def optional_datetime(value: Optional[str]) -> Optional[datetime]:
    if not value:
        return None
    return datetime.fromisoformat(value)
