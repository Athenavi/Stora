"""
Stora 存储套餐模型 — 定义用户存储容量方案
"""
from shared.models import Base
from sqlalchemy import Column, Integer, BigInteger, String, Boolean, DateTime, Text
from sqlalchemy import func


class StoragePlan(Base):
    """存储套餐"""
    __tablename__ = "storage_plans"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(100), nullable=False, comment="套餐名称")
    description = Column(Text, nullable=True, comment="套餐描述")
    storage_bytes = Column(BigInteger, default=1073741824, comment="存储空间 (字节)")
    max_file_size = Column(BigInteger, default=104857600, comment="单文件大小限制")
    max_files_count = Column(Integer, default=10000, comment="最大文件数")
    price = Column(Integer, default=0, comment="价格 (分)")
    is_active = Column(Boolean, default=True)
    sort_order = Column(Integer, default=0)
    created_at = Column(DateTime, default=func.now())
    updated_at = Column(DateTime, default=func.now(), onupdate=func.now())
