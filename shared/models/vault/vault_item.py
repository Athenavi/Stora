"""
SQLAlchemy 模型定义 - VaultItem
由代码生成器自动生成 - 请勿手动修改
"""
from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index
from shared.models import Base


class VaultItem(Base):
    """私密空间文件记录模型"""
    __tablename__ = 'vault_items'

    __table_args__ = (
        Index('idx_vault_items_vault', 'vault_id'),
        Index('idx_vault_items_user', 'user_id'),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='文件 ID')
    vault_id = Column(BigInteger, ForeignKey('vaults.id'), doc='所属 Vault')
    user_id = Column(BigInteger, ForeignKey('users.id'), doc='所有者')
    filename_encrypted = Column(String(512), nullable=True, doc='加密后的文件名(hex)')
    filename_nonce = Column(String(32), nullable=True, doc='文件名加密 nonce(hex)')
    mime_type = Column(String(100), nullable=True, doc='MIME 类型')
    file_size = Column(BigInteger, default=0, doc='明文文件大小')
    encrypted_size = Column(BigInteger, default=0, doc='密文文件大小')
    storage_path = Column(String(500), nullable=True, doc='加密文件存储路径')
    file_nonce = Column(String(32), nullable=True, doc='文件内容加密 nonce(hex)')
    created_at = Column(String(255), nullable=True, doc='创建时间')

    def to_dict(self, exclude_sensitive=True):
        data = {
            'id': self.id, 'vault_id': self.vault_id, 'user_id': self.user_id,
            'mime_type': self.mime_type, 'file_size': self.file_size,
            'encrypted_size': self.encrypted_size, 'storage_path': self.storage_path,
            'created_at': self.created_at,
        }
        return data
