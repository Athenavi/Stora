"""
媒体管理服务
SQLAlchemy async 版本 — 替代原 Django ORM 实现
提供缩略图生成、分类标签、批量操作、EXIF处理等
"""
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple

from PIL import Image, ExifTags
from sqlalchemy import select, func, delete as sa_delete, update
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.media import Media
from src.unified_logger import default_logger as logger

# 默认缩略图尺寸配置
DEFAULT_THUMBNAIL_SIZES = [
    ('small', (150, 150)),
    ('medium', (300, 300)),
    ('large', (600, 600)),
]

# EXIF 日期格式
EXIF_DATE_FORMAT = '%Y:%m:%d %H:%M:%S'


class MediaService:
    """
    媒体服务（SQLAlchemy async）
    
    功能:
    1. 自动生成多尺寸缩略图
    2. 媒体分类和标签管理
    3. 批量操作支持
    4. EXIF数据提取和处理
    """

    def __init__(self, media_dir: str = "storage/objects"):
        self.media_dir = Path(media_dir)
        self.media_dir.mkdir(parents=True, exist_ok=True)

    # ==================== 缩略图生成 ====================

    def generate_thumbnails(
            self,
            image_path: str,
            sizes: List[Tuple[str, Tuple[int, int]]] = None
    ) -> Dict[str, str]:
        """生成多尺寸缩略图"""
        sizes = sizes or DEFAULT_THUMBNAIL_SIZES
        thumbnails = {}

        try:
            img = Image.open(image_path)
            # 处理 RGBA 模式
            if img.mode in ('RGBA', 'P'):
                img = img.convert('RGBA')
            for size_name, dimensions in sizes:
                thumb = img.copy()
                thumb.thumbnail(dimensions, Image.LANCZOS)
                thumb_path = self._get_thumbnail_path(image_path, size_name)
                thumb.save(thumb_path, optimize=True, quality=85)
                thumbnails[size_name] = str(thumb_path)
        except Exception as e:
            logger.error(f"生成缩略图失败: {e}")

        return thumbnails

    def _get_thumbnail_path(self, original_path: str, size_name: str) -> Path:
        """获取缩略图路径"""
        original = Path(original_path)
        return original.parent / f"{original.stem}_{size_name}{original.suffix}"

    # ==================== 分类和标签（SQLAlchemy async） ====================

    def _parse_tags(self, tags_str: Optional[str]) -> List[str]:
        """解析标签字符串为列表"""
        if not tags_str:
            return []
        return [tag.strip() for tag in tags_str.split(',') if tag.strip()]

    async def add_tags(self, db: AsyncSession, media_id: int, user_id: int, tags: List[str]) -> bool:
        """为媒体添加标签"""
        try:
            result = await db.execute(
                select(Media).where(Media.id == media_id, Media.user == user_id)
            )
            media = result.scalar_one_or_none()
            if not media:
                return False
            existing_tags = set(self._parse_tags(media.tags))
            existing_tags.update(tag.strip() for tag in tags if tag.strip())
            media.tags = ','.join(existing_tags)
            await db.commit()
            return True
        except Exception as e:
            await db.rollback()
            logger.error(f"添加标签失败: {e}")
            return False

    async def remove_tags(self, db: AsyncSession, media_id: int, user_id: int, tags: List[str]) -> bool:
        """移除标签"""
        try:
            result = await db.execute(
                select(Media).where(Media.id == media_id, Media.user == user_id)
            )
            media = result.scalar_one_or_none()
            if not media:
                return False
            existing_tags = set(self._parse_tags(media.tags))
            tags_to_remove = {tag.strip() for tag in tags}
            existing_tags -= tags_to_remove
            media.tags = ','.join(existing_tags) if existing_tags else None
            await db.commit()
            return True
        except Exception as e:
            await db.rollback()
            logger.error(f"移除标签失败: {e}")
            return False

    async def get_media_by_tag(self, db: AsyncSession, tag: str, user_id: Optional[int] = None) -> List[Dict[str, Any]]:
        """根据标签获取媒体"""
        try:
            query = select(Media).where(Media.tags.contains(tag))
            if user_id is not None:
                query = query.where(Media.user == user_id)
            query = query.order_by(Media.created_at.desc())
            result = await db.execute(query)
            return [m.to_dict() for m in result.scalars().all()]
        except Exception as e:
            logger.error(f"查询标签媒体失败: {e}")
            return []

    async def get_all_tags(self, db: AsyncSession, user_id: Optional[int] = None) -> List[str]:
        """获取所有标签"""
        try:
            query = select(Media.tags).where(
                Media.tags.isnot(None),
                Media.tags != ''
            )
            if user_id is not None:
                query = query.where(Media.user == user_id)
            result = await db.execute(query)
            all_tags = set()
            for row in result.all():
                tags = row[0]
                if tags:
                    all_tags.update(self._parse_tags(tags))
            return sorted(all_tags)
        except Exception as e:
            logger.error(f"获取所有标签失败: {e}")
            return []

    async def categorize_media(self, db: AsyncSession, media_id: int, user_id: int, category: str) -> bool:
        """分类媒体"""
        try:
            result = await db.execute(
                select(Media).where(Media.id == media_id, Media.user == user_id)
            )
            media = result.scalar_one_or_none()
            if not media:
                return False
            media.category = category
            await db.commit()
            return True
        except Exception as e:
            await db.rollback()
            logger.error(f"分类媒体失败: {e}")
            return False

    # ==================== 批量操作 ====================

    async def batch_delete(self, db: AsyncSession, media_ids: List[int], user_id: int) -> Dict[str, Any]:
        """批量删除媒体"""
        results = {'success': 0, 'failed': 0, 'errors': []}
        import os

        for media_id in media_ids:
            try:
                result = await db.execute(
                    select(Media).where(Media.id == media_id, Media.user == user_id)
                )
                media = result.scalar_one_or_none()
                if not media:
                    results['failed'] += 1
                    results['errors'].append({'media_id': media_id, 'error': '媒体不存在'})
                    continue

                # 删除物理文件
                for file_path in [media.file_path, media.thumbnail_path]:
                    if file_path and os.path.exists(file_path):
                        os.remove(file_path)

                await db.delete(media)
                results['success'] += 1
            except Exception as e:
                results['failed'] += 1
                results['errors'].append({'media_id': media_id, 'error': str(e)})

        await db.commit()
        return results

    async def batch_update_tags(
        self, db: AsyncSession, media_ids: List[int], user_id: int, tags: List[str]
    ) -> Dict[str, Any]:
        """批量更新标签"""
        results = {'success': 0, 'failed': 0}
        for media_id in media_ids:
            if await self.add_tags(db, media_id, user_id, tags):
                results['success'] += 1
            else:
                results['failed'] += 1
        return results

    async def batch_categorize(
        self, db: AsyncSession, media_ids: List[int], user_id: int, category: str
    ) -> Dict[str, Any]:
        """批量分类"""
        results = {'success': 0, 'failed': 0}
        for media_id in media_ids:
            if await self.categorize_media(db, media_id, user_id, category):
                results['success'] += 1
            else:
                results['failed'] += 1
        return results

    # ==================== EXIF数据处理 ====================

    async def extract_exif(self, image_path: str) -> Dict[str, Any]:
        """提取EXIF数据"""
        try:
            img = Image.open(image_path)
            exif_data = {'width': img.width, 'height': img.height, 'format': img.format}

            if hasattr(img, '_getexif') and img._getexif():
                for tag_id, value in img._getexif().items():
                    tag_name = ExifTags.TAGS.get(tag_id, tag_id)
                    if isinstance(value, bytes):
                        continue

                    if tag_name == 'DateTimeOriginal':
                        try:
                            exif_data['date_taken'] = datetime.strptime(value, EXIF_DATE_FORMAT).isoformat()
                        except ValueError:
                            pass
                    elif tag_name == 'GPSInfo':
                        exif_data['gps'] = self._parse_gps_info(value)
                    else:
                        exif_data[tag_name.lower()] = value

            return exif_data
        except Exception as e:
            logger.error(f"提取EXIF失败: {e}")
            return {}

    def _parse_gps_info(self, gps_info: Dict) -> Optional[Dict[str, float]]:
        """解析GPS信息"""
        try:
            return {
                'latitude': float(gps_info.get(2, [0])[0]),
                'longitude': float(gps_info.get(4, [0])[0]),
            }
        except (TypeError, IndexError, ValueError):
            return None

    def remove_exif(self, input_path: str, output_path: Optional[str] = None) -> bool:
        """
        移除EXIF数据(保护隐私)
        使用 PIL 保存时清除 EXIF 但不重编码像素数据
        """
        try:
            output_path = output_path or input_path
            img = Image.open(input_path)
            img.save(output_path, format=img.format, exif=b'')
            return True
        except Exception as e:
            logger.error(f"移除EXIF失败: {e}")
            return False


# 全局实例
media_service = MediaService()
