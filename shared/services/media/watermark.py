"""
Stora 文件水印 — 预览时自动叠加水印 (图片/PDF)
"""
import os
from typing import Optional

from PIL import Image, ImageDraw, ImageFont
from src.setting import settings


def get_watermark_config() -> dict:
    """获取水印配置"""
    return {
        "text": getattr(settings, "WATERMARK_TEXT", os.environ.get("WATERMARK_TEXT", "Stora")),
        "opacity": int(os.environ.get("WATERMARK_OPACITY", "30")),
        "size": int(os.environ.get("WATERMARK_SIZE", "36")),
        "position": os.environ.get("WATERMARK_POSITION", "center"),  # center / tile
        "enabled": os.environ.get("WATERMARK_ENABLED", "False").lower() == "true",
    }


def apply_image_watermark(image_path: str, output_path: Optional[str] = None) -> str:
    """
    给图片叠加水印，返回输出路径
    如果水印未启用，直接返回原路径
    """
    config = get_watermark_config()
    if not config["enabled"]:
        return image_path

    img = Image.open(image_path).convert("RGBA")
    txt_layer = Image.new("RGBA", img.size, (255, 255, 255, 0))
    draw = ImageDraw.Draw(txt_layer)

    try:
        font = ImageFont.truetype("arial.ttf", config["size"])
    except (OSError, IOError):
        font = ImageFont.load_default()

    text = config["text"]
    opacity = config["opacity"]
    position = config["position"]

    if position == "tile":
        # 平铺水印
        bbox = draw.textbbox((0, 0), text, font=font)
        tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
        y = 0
        while y < img.height:
            x = 0
            while x < img.width:
                draw.text((x, y), text, font=font, fill=(255, 255, 255, opacity))
                x += tw + 40
            y += th + 40
    else:
        # 居中水印（倾斜45度）
        bbox = draw.textbbox((0, 0), text, font=font)
        tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
        cx, cy = img.width // 2, img.height // 2
        txt_layer_rot = txt_layer.rotate(45, expand=1)
        draw_rot = ImageDraw.Draw(txt_layer_rot)
        draw_rot.text((0, 0), text, font=font, fill=(255, 255, 255, opacity))
        txt_layer_rot = txt_layer_rot.rotate(-45, expand=1)
        tx = cx - tw // 2
        ty = cy - th // 2
        txt_layer.paste(txt_layer_rot, (tx, ty))

    watermarked = Image.alpha_composite(img, txt_layer).convert("RGB")

    output_path = output_path or image_path
    watermarked.save(output_path, "JPEG", quality=90)
    return output_path


def apply_pdf_watermark(pdf_path: str, output_path: Optional[str] = None) -> str:
    """
    给 PDF 叠加水印（使用 pypdf 在每页叠加文字）
    如果水印未启用或 pypdf 不可用，直接返回原路径
    """
    config = get_watermark_config()
    if not config["enabled"]:
        return pdf_path

    try:
        from pypdf import PdfReader, PdfWriter
        from reportlab.pdfgen import canvas
        from reportlab.lib.pagesizes import letter
        import io
    except ImportError:
        return pdf_path

    reader = PdfReader(pdf_path)
    writer = PdfWriter()

    for page_num in range(len(reader.pages)):
        page = reader.pages[page_num]
        packet = io.BytesIO()
        c = canvas.Canvas(packet, pagesize=letter)
        c.setFont("Helvetica", config["size"])
        c.setFillAlpha(config["opacity"] / 100)
        c.saveState()
        c.translate(letter[0] / 2, letter[1] / 2)
        c.rotate(45)
        c.drawCentredString(0, 0, config["text"])
        c.restoreState()
        c.save()

        packet.seek(0)
        watermark = PdfReader(packet)
        page.merge_page(watermark.pages[0])
        writer.add_page(page)

    out = output_path or pdf_path
    with open(out, "wb") as f:
        writer.write(f)

    return out
