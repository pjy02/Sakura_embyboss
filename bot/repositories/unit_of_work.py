from typing import Optional

from sqlalchemy.orm import Session as OrmSession

from bot.sql_helper import Session
from bot.repositories.auth import AuthRepository
from bot.repositories.codes import CodeRepository
from bot.repositories.commerce import CommerceRepository
from bot.repositories.community import CommunityRepository
from bot.repositories.core_operations import CoreOperationsRepository
from bot.repositories.operations import OperationRepository
from bot.repositories.partitions import PartitionRepository
from bot.repositories.users import UserRepository


class SqlAlchemyUnitOfWork:
    """One transaction shared by all repositories in a business operation."""

    def __init__(self, session_factory=Session):
        self._session_factory = session_factory
        self.session: Optional[OrmSession] = None

    def __enter__(self) -> "SqlAlchemyUnitOfWork":
        self.session = self._session_factory()
        self.auth = AuthRepository(self.session)
        self.users = UserRepository(self.session)
        self.codes = CodeRepository(self.session)
        self.commerce = CommerceRepository(self.session)
        self.community = CommunityRepository(self.session)
        self.core_operations = CoreOperationsRepository(self.session)
        self.partitions = PartitionRepository(self.session)
        self.operations = OperationRepository(self.session)
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        if self.session is None:
            return
        try:
            if exc_type is None:
                self.session.commit()
            else:
                self.session.rollback()
        finally:
            self.session.close()

    def flush(self) -> None:
        if self.session is None:
            raise RuntimeError("Unit of work has not been entered")
        self.session.flush()
