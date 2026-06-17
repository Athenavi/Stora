"""
版本管理器
统一版本管理 — 同时支持 JSON 和旧的 configparser 格式
"""
import json
import os
from datetime import datetime
from pathlib import Path
from typing import Dict, Optional


class VersionManager:
    """版本管理器（JSON 格式优先，向后兼容 configparser）"""

    def __init__(self, version_file_path: Optional[str] = None):
        if version_file_path is None:
            current = Path(os.path.dirname(os.path.abspath(__file__)))
            project_root = current.parent.parent
            self.version_file = project_root / 'version.txt'
        else:
            self.version_file = Path(version_file_path)
        
        self._data = self._load()

    def _load(self) -> dict:
        """加载版本信息（JSON 优先，回退到 configparser）"""
        if not self.version_file.exists():
            return self._create_default()

        raw = self.version_file.read_text(encoding='utf-8')

        # 尝试 JSON 解析
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            pass

        # 回退：configparser 格式
        import configparser
        cp = configparser.ConfigParser()
        try:
            cp.read_string(raw)
            data = {}
            for section in cp.sections():
                data[section.lower()] = dict(cp[section])
            return data
        except Exception:
            return self._create_default()

    def _create_default(self) -> dict:
        data = {
            'release': {'version': '0.1.0', 'build_time': datetime.now().isoformat()},
            'database': {'migration': 'base', 'status': 'up_to_date'},
            'author': {'maintainer': 'Athenavi', 'repository': 'https://github.com/Athenavi/fast_blog'},
        }
        self._save(data)
        return data

    def _save(self, data: dict = None):
        if data is None:
            data = self._data
        self.version_file.write_text(
            json.dumps(data, ensure_ascii=False, indent=2),
            encoding='utf-8'
        )

    # ── 读取 ──

    def get_version(self) -> str:
        return self._data.get('release', {}).get('version', '0.0.0')

    def get_release_info(self) -> Dict[str, str]:
        return self._data.get('release', {})

    def get_database_info(self) -> Dict[str, str]:
        return self._data.get('database', {})

    def get_author_info(self) -> Dict[str, str]:
        return self._data.get('author', {})

    def get_all(self) -> Dict[str, Dict[str, str]]:
        return {
            'release': self.get_release_info(),
            'database': self.get_database_info(),
            'author': self.get_author_info(),
        }

    def get_backend_version(self) -> dict:
        return self._data.get('release', {})

    def get_frontend_version(self) -> dict:
        return self._data.get('release', {})

    def get_all_versions(self) -> dict:
        return {'BACKEND': self.get_backend_version(), 'FRONTEND': self.get_frontend_version()}

    # ── 写入 ──

    def bump_version(self, version: str):
        self._data.setdefault('release', {})['version'] = version
        self._data['release']['build_time'] = datetime.now().isoformat()
        self._save()

    def update_database(self, migration: str, status: str = 'up_to_date'):
        self._data.setdefault('database', {})['migration'] = migration
        self._data['database']['status'] = status
        self._save()


version_manager = VersionManager()
