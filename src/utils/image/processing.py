"""图像处理工具模块"""
import io
from typing import Tuple, Optional

from PIL import Image

# 限制最大像素数，防止 decompression bomb 攻击（作用于本模块所有 Image.open 调用）
Image.MAX_IMAGE_PIXELS = 100_000_000

from shared.models import FileHash


def resize_image(input_path: str, output_path: str, max_size: Tuple[int, int] = (1920, 1080), quality: int = 85):
    """
    调整图像大小
    
    Args:
        input_path: 输入图像路径
        output_path: 输出图像路径
        max_size: 最大尺寸 (width, height)
        quality: JPEG质量 (1-100)
    """
    with Image.open(input_path) as img:
        # 保持宽高比进行缩放
        img.thumbnail(max_size, Image.Resampling.LANCZOS)

        # 保存图像
        img.save(output_path, optimize=True, quality=quality)


def optimize_image(input_path: str, output_path: str, quality: int = 85, max_size: Optional[Tuple[int, int]] = None):
    """
    优化图像大小和质量
    
    Args:
        input_path: 输入图像路径
        output_path: 输出图像路径
        quality: JPEG质量 (1-100)
        max_size: 最大尺寸 (width, height)，如果提供则会缩放
    """
    with Image.open(input_path) as img:
        if max_size:
            img.thumbnail(max_size, Image.Resampling.LANCZOS)

        # 确定输出格式
        output_format = img.format or 'JPEG'

        # 保存优化后的图像
        img.save(output_path, format=output_format, optimize=True, quality=quality)


def get_image_info(image_path: str) -> dict:
    """
    获取图像信息
    
    Args:
        image_path: 图像路径
    
    Returns:
        包含图像信息的字典
    """
    with Image.open(image_path) as img:
        return {
            'format': img.format,
            'mode': img.mode,
            'size': img.size,
            'width': img.width,
            'height': img.height
        }


def crop_image(input_path: str, output_path: str, box: Tuple[int, int, int, int]):
    """
    裁剪图像
    
    Args:
        input_path: 输入图像路径
        output_path: 输出图像路径
        box: 裁剪框 (left, top, right, bottom)
    """
    with Image.open(input_path) as img:
        cropped = img.crop(box)
        cropped.save(output_path)


def convert_image_format(input_path: str, output_path: str, format: str = 'JPEG'):
    """
    转换图像格式
    
    Args:
        input_path: 输入图像路径
        output_path: 输出图像路径
        format: 输出格式 ('JPEG', 'PNG', 'WEBP' 等)
    """
    with Image.open(input_path) as img:
        img.save(output_path, format=format)


def create_video_thumbnail(input_path: str, output_path: str, time_offset: int = 1) -> bool:
    """
    创建视频缩略图（优先使用 ffmpeg，降级到 opencv）

    Args:
        input_path: 视频文件路径
        output_path: 输出缩略图路径
        time_offset: 截取时间点（秒）

    Returns:
        是否成功
    """
    import subprocess
    import os

    # 方案1: ffmpeg
    try:
        result = subprocess.run(
            ['ffmpeg', '-ss', str(time_offset), '-i', input_path,
             '-vframes', '1', '-q:v', '2', output_path, '-y'],
            capture_output=True, timeout=30
        )
        if result.returncode == 0 and os.path.exists(output_path):
            return True
    except (subprocess.SubprocessError, FileNotFoundError, OSError):
        pass

    # 方案2: opencv (降级)
    try:
        import cv2
        cap = cv2.VideoCapture(input_path)
        if not cap.isOpened():
            return False
        cap.set(cv2.CAP_PROP_POS_FRAMES, int(cap.get(cv2.CAP_PROP_FPS)) * time_offset)
        ret, frame = cap.read()
        cap.release()
        if ret:
            cv2.imwrite(output_path, frame)
            return os.path.exists(output_path)
    except ImportError:
        pass

    return False


async def get_file_mime_type(file_hash: str, db=None) -> str:
    """
    从数据库获取文件的MIME类型
    
    Args:
        file_hash: 文件哈希值
        db: 数据库会话（可选）
    
    Returns:
        MIME类型字符串
    """
    try:
        mime_type = None
        
        # 如果没有提供数据库会话，创建一个新的
        if db is None:
            from src.extensions import get_async_db_session
            from sqlalchemy import select

            # 使用 async with 正确管理数据库会话
            async with get_async_db_session()() as session:
                result_query = select(FileHash.mime_type).where(FileHash.hash == file_hash)
                result_result = await session.execute(result_query)
                result = result_result.first()
                if result:
                    # 查询结果通常是一个元组，取第一个元素
                    mime_type = result[0] if isinstance(result, tuple) else getattr(result, 'mime_type', None)
        else:
            # 如果提供了数据库会话，直接使用
            from sqlalchemy import select

            result_query = select(FileHash.mime_type).where(FileHash.hash == file_hash)
            result_result = await db.execute(result_query)
            result = result_result.first()
            if result:
                # 查询结果通常是一个元组，取第一个元素
                mime_type = result[0] if isinstance(result, tuple) else getattr(result, 'mime_type', None)

        # 如果在数据库中找到了mime_type，直接返回
        if mime_type:
            return mime_type

        # 如果数据库中没有找到对应的mime_type，尝试使用magic库
        import magic
        # 假设文件路径可以通过哈希构建
        file_path = f"storage/objects/{file_hash[:2]}/{file_hash}"
        mime_type = magic.from_file(file_path, mime=True)
        return mime_type
    except Exception as e:
        print(f"Error detecting file type: {str(e)}")
        # 如果所有方法都失败，返回默认的octet-stream类型
        return 'application/octet-stream'


def validate_image(image_path: str) -> Tuple[bool, str]:
    """
    验证图像文件
    
    Args:
        image_path: 图像路径
    
    Returns:
        (是否有效, 错误信息)
    """
    try:
        with Image.open(image_path) as img:
            # 尝试加载图像以验证其有效性
            img.verify()
        return True, ""
    except Exception as e:
        return False, str(e)


def get_image_size_from_bytes(image_bytes: bytes) -> Tuple[int, int]:
    """
    从字节数据获取图像尺寸
    
    Args:
        image_bytes: 图像字节数据
    
    Returns:
        (宽度, 高度)
    """
    img = Image.open(io.BytesIO(image_bytes))
    return img.size


def generate_thumbnail(input_path: str, output_path: str, size: Tuple[int, int] = (200, 200), quality: int = 85):
    """
    生成缩略图
    
    Args:
        input_path: 输入图像路径
        output_path: 输出缩略图路径
        size: 缩略图尺寸，默认为 (200, 200)
        quality: JPEG质量 (1-100)
    """
    with Image.open(input_path) as img:
        # 转换RGBA为RGB（如果需要），以便保存为JPEG
        if img.mode in ('RGBA', 'LA', 'P'):
            # 如果有透明度，替换为白色背景
            if img.mode == 'P':
                # 转换P模式为RGBA
                img = img.convert('RGBA')
            if img.mode in ('RGBA', 'LA'):
                # 创建白色背景
                background = Image.new('RGB', img.size, (255, 255, 255))
                if img.mode == 'RGBA':
                    background.paste(img, mask=img.split()[-1])  # 使用alpha通道作为掩码
                else:
                    background.paste(img)
                img = background

        # 生成缩略图
        img.thumbnail(size, Image.Resampling.LANCZOS)

        # 确保输出格式为JPEG
        if output_path.lower().endswith('.jpg') or output_path.lower().endswith('.jpeg'):
            img.save(output_path, 'JPEG', optimize=True, quality=quality)
        else:
            img.save(output_path, optimize=True, quality=quality)
