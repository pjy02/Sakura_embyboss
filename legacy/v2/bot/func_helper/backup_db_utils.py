import asyncio
import glob
import os
from datetime import datetime

from bot import LOGGER


class BackupDBUtils:

    @staticmethod
    # 数据库备份(mysql直装/本机含有mysql)
    async def backup_mysql_db(host, port, user, password, database_name, backup_dir, max_backup_count):
        # 如果文件夹不存在，就创建它
        if not os.path.exists(backup_dir):
            os.makedirs(backup_dir)
        # 根据时间创建当前备份文件
        backup_file = os.path.join(backup_dir, f'{database_name}-{datetime.now().strftime("%Y-%m-%d-%H-%M-%S")}.sql')
        base_command = [
            "mysqldump", f"-h{host}", "--no-tablespaces", f"-P{port}",
            f"-u{user}", database_name,
        ]
        process_env = {**os.environ, "MYSQL_PWD": str(password)}

        async def run_dump(extra_args=None):
            # Avoid a shell so credentials and config values cannot alter the
            # command.  The password is provided through MYSQL_PWD instead of
            # being exposed in the process argument list.
            with open(backup_file, "wb") as output:
                process = await asyncio.create_subprocess_exec(
                    *(base_command[:1] + list(extra_args or []) + base_command[1:]),
                    stdout=output,
                    stderr=asyncio.subprocess.PIPE,
                    env=process_env,
                )
                _stdout, stderr = await process.communicate()
            return process.returncode, (stderr or b"").decode("utf-8", errors="replace")

        try:
            return_code, error_output = await run_dump()
            if return_code != 0:
                LOGGER.warning(f"BOT数据库备份失败，使用 skip-ssl方式尝试备份")
                return_code, error_output = await run_dump(["--skip-ssl"])
            if return_code != 0:
                LOGGER.error(
                    f"BOT数据库备份失败, error code: {return_code}, "
                    f"detail: {error_output[-500:]}"
                )
                try:
                    os.remove(backup_file)
                except FileNotFoundError:
                    pass
                return None
            try:
                os.chmod(backup_file, 0o600)
            except OSError:
                pass
            LOGGER.info(f"BOT数据库备份成功,文件保存为 {backup_file}")
            # 获取所有备份文件，并且通过时间进行排序
            all_backups = sorted(glob.glob(os.path.join(backup_dir, f'{database_name}-*.sql')))
            # 如果超过了当前的备份最大数量，则删除最久的一个
            while len(all_backups) > max_backup_count:
                os.remove(all_backups[0])
                all_backups.pop(0)
        except Exception as e:
            LOGGER.error(f"BOT数据库备份失败, error: {str(e)}")
            return None
        return backup_file

    @staticmethod
    # 数据库备份(docker)
    async def backup_mysql_db_docker(container_name, user, password, database_name, backup_dir, max_backup_count):
        # 如果文件夹不存在，就创建它
        if not os.path.exists(backup_dir):
            os.makedirs(backup_dir)
        # 根据当前时间创建备份文件
        backup_file_in_container = f'{database_name}-{datetime.now().strftime("%Y-%m-%d-%H-%M-%S")}.sql'
        backup_file_on_host = os.path.join(backup_dir, backup_file_in_container)
        # 进入容器，使用mysqldump备份文件
        command = f'docker exec {container_name} sh -c "mysqldump  --no-tablespaces -u{user} -p\'{password}\' {database_name} > {backup_file_in_container}"'
        skip_ssl_command = f'docker exec {container_name} sh -c "mysqldump --skip-ssl --no-tablespaces -u{user} -p\'{password}\' {database_name} > {backup_file_in_container}"'
        return_code = -1
        try:
            process = await asyncio.create_subprocess_shell(command)
            await process.communicate()
            return_code = process.returncode
            if return_code != 0:
                LOGGER.warning(f"BOT数据库备份失败，使用 skip-ssl方式尝试备份")
                process = await asyncio.create_subprocess_shell(skip_ssl_command)
                await process.communicate()
                return_code = process.returncode
            if return_code != 0:
                LOGGER.error(f"BOT数据库备份失败, error code: {return_code}")
                return None
            # 将容器中的备份文件复制到本地
            command = f'docker cp {container_name}:{backup_file_in_container} {backup_file_on_host}'
            process = await asyncio.create_subprocess_shell(command)
            await process.communicate()
        except Exception as e:
            LOGGER.error(f"BOT数据库备份失败, error: {str(e)}")
        finally:
            # 删除容器中文件
            command = f'docker exec {container_name} rm {backup_file_in_container}'
            process = await asyncio.create_subprocess_shell(command)
            await process.communicate()
        LOGGER.info(f"BOT数据库备份成功,文件保存为 {backup_file_on_host}")
        # 获取所有备份文件，并且通过时间进行排序
        all_backups = sorted(glob.glob(os.path.join(backup_dir, f'{database_name}-*.sql')))
        # 如果超过了当前的备份最大数量，则删除最久的一个
        while len(all_backups) > max_backup_count:
            os.remove(all_backups[0])
            all_backups.pop(0)
        return backup_file_on_host
