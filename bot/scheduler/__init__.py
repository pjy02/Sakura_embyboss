from bot import LOGGER
from bot.application import DynamicSettingsService

try:
    DynamicSettingsService().apply_runtime_overrides()
except Exception as error:
    LOGGER.warning(f"动态设置初始加载失败，继续使用 config.json：{error}")

from .userplays_rank import Uplaysinfo
from .backup_db import DbBackupUtils
from .bot_commands import BotCommands
from .check_ex import check_expired
from .check_restart import check_restart
from .ranks_task import week_ranks, day_ranks
from .sync_favorites import sync_favorites
from .sync_mp_download import sync_download_tasks
from .sync_core_operations import sync_core_operations
from .partition_access import check_partition_access
from .sync_dynamic_settings import sync_dynamic_settings
