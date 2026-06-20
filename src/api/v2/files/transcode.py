"""
Stora Transcode API — 视频转码任务管理
利用 video_processor.py 中的 VideoProcessor 类进行 ffmpeg 转码。
"""
import json
import os
from datetime import datetime

from fastapi import APIRouter, BackgroundTasks, Depends
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import User
from shared.models.file.file_item import FileItem
from shared.models.file.transcode_task import TranscodeTask
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db
from src.utils.image.video_processor import video_processor

router = APIRouter(prefix="/transcode", tags=["transcode"])

TRANSCODE_DIR = os.path.join(os.getcwd(), "storage", "transcodes")
os.makedirs(TRANSCODE_DIR, exist_ok=True)


async def _do_transcode(task_id: int, file_id: int, file_path: str, user_id: int):
    """后台执行转码"""
    from src.extensions import get_async_session_context
    from shared.models.file.transcode_task import TranscodeTask as TT

    async with get_async_session_context() as db:
        task = await db.get(TT, task_id)
        if not task:
            return

        task.status = "processing"
        task.progress = 5
        await db.commit()

        try:
            # Get original video info
            info = video_processor.get_video_info(file_path)
            if not info:
                task.status = "failed"
                task.error_message = "无法获取视频信息"
                await db.commit()
                return

            source_res = f"{info['width']}x{info['height']}"
            task.source_resolution = source_res
            task.progress = 10
            await db.commit()

            # Generate multi-resolution outputs
            output_dir = os.path.join(TRANSCODE_DIR, str(task_id))
            os.makedirs(output_dir, exist_ok=True)

            resolutions = [
                {'width': 1920, 'height': 1080, 'label': '1080p'},
                {'width': 1280, 'height': 720, 'label': '720p'},
                {'width': 854, 'height': 480, 'label': '480p'},
            ]

            # Only include resolutions smaller than source
            valid_res = [r for r in resolutions
                         if r['width'] <= info['width'] or r['height'] <= info['height']]

            results = []
            for i, res in enumerate(valid_res):
                output_filename = f"{res['label']}.mp4"
                output_path = os.path.join(output_dir, output_filename)

                result = video_processor.transcode_video(
                    input_path=file_path,
                    output_path=output_path,
                    max_width=res['width'],
                    max_height=res['height'],
                    preset='fast',
                    crf=23,
                )

                if result.get('success'):
                    results.append({
                        'label': res['label'],
                        'path': output_path,
                        'size': result.get('transcoded_size', 0),
                        'width': res['width'],
                        'height': res['height'],
                    })

                task.progress = 20 + int((i + 1) / len(valid_res) * 70)
                await db.commit()

            task.output_files = json.dumps(results)
            task.status = "completed" if results else "failed"
            task.progress = 100
            if not results:
                task.error_message = "所有分辨率转码均失败"
            await db.commit()

        except Exception as exc:
            task.status = "failed"
            task.error_message = str(exc)
            await db.commit()


@router.post("/{file_id}")
async def create_transcode(
    file_id: int,
    background_tasks: BackgroundTasks,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """创建视频转码任务"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    if file.file_type != "video":
        return fail("仅支持视频文件转码")

    # Check ffmpeg availability
    if not video_processor._check_ffmpeg():
        return fail("服务器未安装 ffmpeg，无法转码")

    # Check existing active task
    existing = (await db.execute(
        select(TranscodeTask).where(
            TranscodeTask.file_id == file_id,
            TranscodeTask.status.in_(["pending", "processing"]),
        )
    )).scalar_one_or_none()
    if existing:
        return ok({"task_id": existing.id, "status": existing.status}, msg="已有进行中的转码任务")

    src_path = getattr(file, "file_path", None) or getattr(file, "storage_path", None)
    if not src_path or not os.path.exists(src_path):
        return fail("源文件不存在")

    task = TranscodeTask(
        file_id=file_id,
        user_id=current_user.id,
        status="pending",
    )
    db.add(task)
    await db.commit()
    await db.refresh(task)

    background_tasks.add_task(_do_transcode, task.id, file_id, src_path, current_user.id)

    return ok({"task_id": task.id, "status": task.status})


@router.get("/{file_id}/tasks")
async def list_transcode_tasks(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出文件的转码任务"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")

    tasks = (await db.execute(
        select(TranscodeTask).where(
            TranscodeTask.file_id == file_id,
        ).order_by(TranscodeTask.created_at.desc())
    )).scalars().all()

    return ok([t.to_dict() for t in tasks])


@router.get("/{file_id}/tasks/{task_id}")
async def get_transcode_task(
    file_id: int,
    task_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """查询单个转码任务状态"""
    task = await db.get(TranscodeTask, task_id)
    if not task or task.file_id != file_id or task.user_id != current_user.id:
        return fail("任务不存在")
    return ok(task.to_dict())
