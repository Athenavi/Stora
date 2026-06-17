"""
数据库迁移管理器
基于Alembic实现自动数据库迁移
"""
import os
import sys
from pathlib import Path
from typing import Dict, Any, List, Optional


class MigrationManager:
    """基于Alembic的数据库迁移管理器"""
    
    def __init__(self):
        self.alembic_ini = Path(__file__).resolve().parent.parent.parent.parent / "alembic.ini"
        self.migrations_dir = Path(__file__).resolve().parent.parent.parent.parent / "alembic_migrations"
        self.versions_dir = self.migrations_dir / "versions"

    def _get_alembic_cfg(self) -> "alembic.config.Config":
        """获取 Alembic 配置对象"""
        from alembic.config import Config
        cfg = Config(str(self.alembic_ini))
        # 从 app_config 同步数据库 URL
        try:
            from src.setting import app_config
            if app_config.database_url:
                cfg.set_main_option("sqlalchemy.url", app_config.database_url)
        except Exception:
            pass
        return cfg
    
    def _run_alembic(self, args: List[str], cwd: Optional[Path] = None) -> Dict[str, Any]:
        """执行Alembic命令（使用 Python API 避免子进程问题）"""
        from io import StringIO
        import traceback
        from alembic import command

        cfg = self._get_alembic_cfg()

        # 重定向 stdout/stderr 以捕获输出
        old_stdout = sys.stdout
        old_stderr = sys.stderr
        captured_stdout = StringIO()
        captured_stderr = StringIO()
        sys.stdout = captured_stdout
        sys.stderr = captured_stderr

        try:
            subcommand = args[0] if args else ''
            if subcommand == 'upgrade':
                revision = args[1] if len(args) > 1 else 'head'
                command.upgrade(cfg, revision)
            elif subcommand == 'downgrade':
                revision = args[1] if len(args) > 1 else '-1'
                command.downgrade(cfg, revision)
            elif subcommand == 'current':
                command.current(cfg)
            elif subcommand == 'history':
                command.history(cfg)
            elif subcommand == 'revision':
                message = ''
                autogenerate = False
                for i, a in enumerate(args):
                    if a == '-m' and i + 1 < len(args):
                        message = args[i + 1]
                    if a == '--autogenerate':
                        autogenerate = True
                command.revision(cfg, message=message, autogenerate=autogenerate)
            else:
                raise ValueError(f"Unknown alembic subcommand: {subcommand}")

            return {
                'success': True,
                'stdout': captured_stdout.getvalue(),
                'stderr': captured_stderr.getvalue(),
                'returncode': 0,
            }
        except SystemExit as e:
            return {
                'success': e.code == 0 if e.code is not None else True,
                'stdout': captured_stdout.getvalue(),
                'stderr': captured_stderr.getvalue(),
                'returncode': e.code or 0,
            }
        except Exception as e:
            return {
                'success': False,
                'stdout': captured_stdout.getvalue(),
                'stderr': captured_stderr.getvalue() + '\n' + traceback.format_exc(),
                'returncode': -1,
            }
        finally:
            sys.stdout = old_stdout
            sys.stderr = old_stderr
    
    def get_current_revision(self) -> Dict[str, Any]:
        """
        获取当前数据库版本（直接查 alembic_version 表）

        Returns:
            当前版本信息
        """
        from sqlalchemy import create_engine, text
        try:
            cfg = self._get_alembic_cfg()
            url = cfg.get_main_option("sqlalchemy.url")
            engine = create_engine(url)
            with engine.connect() as conn:
                result = conn.execute(text("SELECT version_num FROM alembic_version"))
                row = result.fetchone()
                current_rev = row[0] if row else None
            engine.dispose()
            return {
                'success': True,
                'current_revision': current_rev,
            }
        except Exception as e:
            return {
                'success': False,
                'error': str(e),
            }
    
    def get_pending_migrations(self) -> Dict[str, Any]:
        """
        获取待执行的迁移（通过比较当前版本和版本目录）

        Returns:
            待执行迁移列表
        """
        try:
            cfg = self._get_alembic_cfg()
            from alembic.script import ScriptDirectory
            script = ScriptDirectory.from_config(cfg)
            current_rev = self.get_current_revision().get('current_revision')

            heads = script.get_heads()
            if not heads:
                return {'success': True, 'pending_count': 0, 'migrations': []}

            if not current_rev:
                # 无版本记录 → 全部待执行
                pending = list(script.walk_revisions(base='base', head=heads[0]))
                return {
                    'success': True,
                    'pending_count': len(pending),
                    'migrations': [{'revision': r.revision, 'message': r.doc} for r in pending],
                }

            # 从当前版本到最新 head 的待执行列表
            pending = []
            for rev in script.walk_revisions(head=heads[0], base=current_rev):
                if rev.revision != current_rev:
                    pending.append({'revision': rev.revision, 'message': rev.doc})
            pending.reverse()

            return {
                'success': True,
                'pending_count': len(pending),
                'migrations': pending,
            }
        except Exception as e:
            return {
                'success': False,
                'error': str(e),
            }
    
    async def apply_all_migrations(self, db_session=None) -> Dict[str, Any]:
        """
        执行所有待处理的迁移
        
        Args:
            db_session: 数据库会话(可选,Alembic会自行处理)
            
        Returns:
            执行结果
        """
        result = self._run_alembic(['upgrade', 'head'])
        
        if result['success']:
            return {
                'success': True,
                'message': 'All migrations applied successfully',
                'output': result.get('stdout', ''),
                'applied_count': 1,
                'total_pending': 0,
                'results': [],
            }
        else:
            stderr = result.get('stderr', '')
            stdout = result.get('stdout', '')
            rc = result.get('returncode', -1)
            detail = stderr or stdout or f'Exit code {rc}'
            return {
                'success': False,
                'error': f'Migration failed (rc={rc}): {detail[:500]}',
                'output': result['stdout'],
            }
    
    def create_migration(self, message: str, autogenerate: bool = False) -> Dict[str, Any]:
        """
        创建新的迁移文件
        
        Args:
            message: 迁移描述
            autogenerate: 是否自动生成(基于模型变化)
            
        Returns:
            创建结果
        """
        args = ['revision', '-m', message]
        
        if autogenerate:
            args.append('--autogenerate')
        
        result = self._run_alembic(args)
        
        if result['success']:
            # 提取生成的文件名
            lines = result['stdout'].strip().split('\n')
            generated_file = None
            
            for line in lines:
                if 'Generating' in line:
                    generated_file = line.split()[-1]
                    break
            
            return {
                'success': True,
                'message': f'Migration created: {message}',
                'file': generated_file,
                'output': result['stdout'],
            }
        else:
            return {
                'success': False,
                'error': result.get('stderr', 'Failed to create migration'),
            }
    
    def rollback_migration(self, steps: int = 1) -> Dict[str, Any]:
        """
        回滚迁移
        
        Args:
            steps: 回滚步数(-1表示回滚到base)
            
        Returns:
            回滚结果
        """
        target = f'-{steps}' if steps > 0 else 'base'
        result = self._run_alembic(['downgrade', target])
        
        if result['success']:
            return {
                'success': True,
                'message': f'Rolled back {steps} migration(s)',
                'output': result['stdout'],
            }
        else:
            return {
                'success': False,
                'error': result.get('stderr', 'Rollback failed'),
            }
    
    def get_migration_status(self) -> Dict[str, Any]:
        """
        获取迁移状态摘要
        
        Returns:
            状态信息
        """
        current = self.get_current_revision()
        history = self.get_pending_migrations()
        
        return {
            'success': current['success'] and history['success'],
            'current_revision': current.get('current_revision'),
            'has_pending': history.get('pending_count', 0) > 0,
            'pending_count': history.get('pending_count', 0),
            'pending_migrations': history.get('migrations', []),
        }
    
    def stamp_revision(self, revision: str) -> Dict[str, Any]:
        """
        标记数据库为指定版本(不执行迁移)
        
        Args:
            revision: 目标版本号
            
        Returns:
            操作结果
        """
        result = self._run_alembic(['stamp', revision])
        
        if result['success']:
            return {
                'success': True,
                'message': f'Database stamped to {revision}',
            }
        else:
            return {
                'success': False,
                'error': result.get('stderr', 'Stamp failed'),
            }


# 单例实例
migration_manager = MigrationManager()
