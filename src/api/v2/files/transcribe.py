"""
Stora Transcribe API — 语音转字幕

端点：
  POST /files/transcribe/{file_id}  → 触发字幕生成
  GET  /files/transcribe/{file_id}/status → 轮询进度
  GET  /files/transcribe/{file_id}/subtitle → 下载字幕文件

支持两种后端：
  本地：faster-whisper（推荐，需要首次下载模型 ~2GB）
  API：OpenAI Whisper API（设置 OPENAI_API_KEY）
"""
import os
import json
import subprocess
from datetime import datetime
from typing import Optional

from fastapi import APIRouter, BackgroundTasks, Depends, HTTPException
from fastapi.responses import FileResponse
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func

from shared.models import User, FileItem
from shared.models.file.transcription_task import TranscriptionTask
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/transcribe", tags=["transcribe"])


def _get_whisper_model_size() -> str:
    """从环境变量获取 Whisper 模型大小"""
    return os.environ.get("WHISPER_MODEL_SIZE", "base")


def _get_openai_api_key() -> Optional[str]:
    return os.environ.get("OPENAI_API_KEY")


def _check_ffmpeg() -> bool:
    """检查 ffmpeg 是否可用（复用云转码检测逻辑）"""
    try:
        subprocess.run(["ffmpeg", "-version"], capture_output=True, check=True)
        return True
    except Exception:
        return False


async def _do_transcribe(task_id: int, file_path: str, file_id: int):
    """
    后台执行语音转字幕

    策略：
    1. 如果有 OPENAI_API_KEY → 使用 OpenAI Whisper API
    2. 否则尝试 faster-whisper（需要 pip install faster-whisper）
    3. 都不可用 → 标记失败
    """
    from src.extensions import get_async_session_context
    from shared.models.file.transcription_task import TranscriptionTask as TT

    async with get_async_session_context() as db:
        try:
            task = await db.get(TT, task_id)
            if not task:
                return

            task.status = "processing"
            task.progress = 10
            await db.commit()

            # 提取音频
            audio_path = file_path + ".audio.wav"
            if not _check_ffmpeg():
                task.status = "failed"
                task.error_message = "ffmpeg 不可用，无法提取音频"
                await db.commit()
                return

            subprocess.run(
                ["ffmpeg", "-i", file_path, "-vn", "-acodec", "pcm_s16le",
                 "-ar", "16000", "-ac", "1", audio_path, "-y"],
                capture_output=True, check=True
            )
            task.progress = 30
            await db.commit()

            # 执行语音识别
            api_key = _get_openai_api_key()
            subtitle_path = file_path + ".srt"

            if api_key:
                result = _transcribe_with_openai(audio_path, api_key)
            else:
                result = _transcribe_with_local(audio_path)

            if result is None:
                task.status = "failed"
                task.error_message = "转录失败：没有可用的语音识别后端（设置 OPENAI_API_KEY 或 pip install faster-whisper）"
                await db.commit()
                return

            segments, detected_lang = result
            task.language = detected_lang
            task.progress = 70

            # 生成 SRT 文件
            with open(subtitle_path, "w", encoding="utf-8") as f:
                for i, seg in enumerate(segments, 1):
                    start = _fmt_srt_time(seg["start"])
                    end = _fmt_srt_time(seg["end"])
                    f.write(f"{i}\n{start} --> {end}\n{seg['text'].strip()}\n\n")

            task.subtitle_path = subtitle_path
            task.subtitle_format = "srt"
            task.status = "completed"
            task.progress = 100
            await db.commit()

            # 清理临时音频
            try:
                os.remove(audio_path)
            except Exception:
                pass

        except Exception as e:
            try:
                task.status = "failed"
                task.error_message = str(e)
                await db.commit()
            except Exception:
                pass


def _transcribe_with_openai(audio_path: str, api_key: str):
    """使用 OpenAI Whisper API 转录"""
    import httpx

    with open(audio_path, "rb") as f:
        files = {"file": ("audio.wav", f, "audio/wav")}
        data = {"model": "whisper-1", "response_format": "verbose_json"}

        resp = httpx.post(
            "https://api.openai.com/v1/audio/transcriptions",
            headers={"Authorization": f"Bearer {api_key}"},
            files=files,
            data=data,
            timeout=300,
        )
        if resp.status_code != 200:
            raise Exception(f"OpenAI API 错误: {resp.text}")

        result = resp.json()
        segments = [
            {"start": s["start"], "end": s["end"], "text": s["text"]}
            for s in result.get("segments", [])
        ]
        return segments, result.get("language", "unknown")


def _transcribe_with_local(audio_path: str):
    """使用 faster-whisper 本地转录"""
    try:
        from faster_whisper import WhisperModel
    except ImportError:
        return None

    model_size = _get_whisper_model_size()
    model = WhisperModel(model_size, device="cpu", compute_type="int8")

    segments, info = model.transcribe(audio_path, beam_size=5)
    result_segments = [
        {"start": s.start, "end": s.end, "text": s.text}
        for s in segments
    ]
    return result_segments, info.language


def _fmt_srt_time(seconds: float) -> str:
    """将秒数格式化为 SRT 时间戳: HH:MM:SS,mmm"""
    h = int(seconds // 3600)
    m = int((seconds % 3600) // 60)
    s = int(seconds % 60)
    ms = int((seconds - int(seconds)) * 1000)
    return f"{h:02d}:{m:02d}:{s:02d},{ms:03d}"


# ─── API Endpoints ───


@router.post("/{file_id}")
async def create_transcription(
    file_id: int,
    background_tasks: BackgroundTasks,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """触发语音转字幕"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")

    if file.file_type != "video" and file.file_type != "audio":
        return fail("仅支持视频和音频文件")

    # 检查是否已有进行中的任务
    existing = (await db.execute(
        select(TranscriptionTask).where(
            TranscriptionTask.file_id == file_id,
            TranscriptionTask.status.in_(["pending", "processing"]),
        )
    )).scalar_one_or_none()
    if existing:
        return fail("该文件已有进行中的转录任务")

    # 检查 ffmpeg 或 OpenAI API Key
    if not _check_ffmpeg():
        return fail("系统未安装 ffmpeg，无法提取音频")

    storage_path = getattr(file, "file_path", None) or getattr(file, "storage_path", None)
    if not storage_path or not os.path.exists(storage_path):
        return fail("文件存储路径不存在")

    task = TranscriptionTask(
        file_id=file_id,
        user_id=current_user.id,
        status="pending",
        created_at=datetime.utcnow(),
    )
    db.add(task)
    await db.commit()
    await db.refresh(task)

    # 后台执行
    background_tasks.add_task(_do_transcribe, task.id, storage_path, file_id)

    return ok({
        "task_id": task.id,
        "status": task.status,
        "message": "转录任务已创建，请轮询状态",
    })


@router.get("/{file_id}/status")
async def get_transcription_status(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取转录任务状态"""
    task = (await db.execute(
        select(TranscriptionTask).where(
            TranscriptionTask.file_id == file_id,
            TranscriptionTask.user_id == current_user.id,
        ).order_by(TranscriptionTask.created_at.desc())
    )).scalar_one_or_none()

    if not task:
        return ok({"available": False, "tasks": []})

    return ok({
        "available": True,
        "task_id": task.id,
        "status": task.status,
        "progress": task.progress,
        "language": task.language,
        "subtitle_format": task.subtitle_format,
        "error_message": task.error_message,
    })


@router.get("/{file_id}/subtitle")
async def get_subtitle(
    file_id: int,
    fmt: str = "srt",
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取生成的字幕文件"""
    task = (await db.execute(
        select(TranscriptionTask).where(
            TranscriptionTask.file_id == file_id,
            TranscriptionTask.user_id == current_user.id,
            TranscriptionTask.status == "completed",
        ).order_by(TranscriptionTask.created_at.desc())
    )).scalar_one_or_none()

    if not task or not task.subtitle_path:
        return fail("字幕文件不存在或尚未生成完成")

    if not os.path.exists(task.subtitle_path):
        return fail("字幕文件已丢失")

    media_type = "text/srt" if fmt == "srt" else "text/vtt"
    filename = f"subtitle_{file_id}.{fmt}"

    return FileResponse(
        path=task.subtitle_path,
        filename=filename,
        media_type=media_type,
    )
