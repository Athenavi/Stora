"""
SQLAlchemy 模型定义 - Vault
由代码生成器自动生成 - 请勿手动修改
"""
from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index
from shared.models import Base


class Vault(Base):
    """私密空间 Vault 模型"""
    __tablename__ = 'vaults'

    __table_args__ = (
        Index('idx_vaults_user', 'user_id'),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='Vault ID')
    user_id = Column(BigInteger, ForeignKey('users.id'), doc='所有者')
    name = Column(String(100), nullable=True, doc='Vault 名称')
    password_hash = Column(String(128), nullable=True, doc='PBKDF2(password + salt)')
    salt = Column(String(64), nullable=True, doc='PBKDF2 盐值(hex)')
    master_key_encrypted = Column(Text, nullable=True, doc='AES-GCM 加密的主密钥(hex)')
    master_key_nonce = Column(String(32), nullable=True, doc='加密主密钥使用的 nonce(hex)')
    lock_timeout = Column(Integer, default=300, doc='自动锁定秒数')
    file_count = Column(Integer, default=0, doc='文件数量(缓存)')
    total_size = Column(BigInteger, default=0, doc='总大小(缓存)')
    created_at = Column(String(255), nullable=True, doc='创建时间')
    updated_at = Column(String(255), nullable=True, doc='更新时间')

    def to_dict(self, exclude_sensitive=True):
        data = {
            'id': self.id, 'user_id': self.user_id, 'name': self.name,
            'lock_timeout': self.lock_timeout, 'file_count': self.file_count,
            'total_size': self.total_size,
            'created_at': self.created_at, 'updated_at': self.updated_at,
        }
        if not exclude_sensitive:
            data.update({
                'password_hash': self.password_hash, 'salt': self.salt,
            })
        return data
