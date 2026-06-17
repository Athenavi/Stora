"""
SQLAlchemy 模型定义 - User
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, Index
from sqlalchemy.orm import relationship

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class User(Base):
    """用户模型模型"""
    __tablename__ = 'users'


    __table_args__ = (
        Index('idx_users_username', 'username', unique=True),
        Index('idx_users_email', 'email', unique=True),
        Index('idx_users_is_active', 'is_active'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='用户 ID')

    username = Column(String(255), unique=True, nullable=True, doc='用户名')

    email = Column(String(255), nullable=True, doc='邮箱')

    password = Column(String(255), nullable=True, doc='密码（哈希后存储）')

    profile_picture = Column(String(255), nullable=True, doc='个人资料图片')

    bio = Column(String(255), nullable=True, doc='个人简介')

    is_active = Column(Boolean, default=True, doc='是否激活')


    is_superuser = Column(Boolean, default=False, doc='是否为超级管理员')


    is_staff = Column(Boolean, default=False, doc='是否为工作人员')


    date_joined = Column(String(255), nullable=True, doc='注册时间')

    last_login_at = Column(String(255), nullable=True, doc='上次登录时间')

    last_login_ip = Column(String(255), nullable=True, doc='上次登录 IP')

    register_ip = Column(String(255), nullable=True, doc='注册 IP')

    locale = Column(String(255), default='zh_CN', doc='语言设置')

    is_2fa_enabled = Column(Boolean, default=False, doc='是否启用双因素认证')


    totp_secret = Column(String(32), nullable=True, doc='TOTP 密钥')

    backup_codes = Column(Text, nullable=True, doc='备用码(JSON格式存储)')


    total_storage = Column(BigInteger, default=1073741824, doc='总存储空间（字节，默认1GB）')


    used_storage = Column(BigInteger, default=0, doc='已用存储空间（字节）')


    # 关系定义
    roles = relationship('Role', secondary='user_role_assignments', back_populates='users', primaryjoin="User.id == user_role_assignments.c.user_id", secondaryjoin="user_role_assignments.c.role_id == Role.id")

    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'username': self.username,
            'email': self.email,
            'password': self.password,
            'profile_picture': self.profile_picture,
            'bio': self.bio,
            'is_active': self.is_active,
            'is_superuser': self.is_superuser,
            'is_staff': self.is_staff,
            'date_joined': self.date_joined,
            'last_login_at': self.last_login_at,
            'last_login_ip': self.last_login_ip,
            'register_ip': self.register_ip,
            'locale': self.locale,
            'is_2fa_enabled': self.is_2fa_enabled,
            'totp_secret': self.totp_secret,
            'backup_codes': self.backup_codes,
            'total_storage': self.total_storage,
            'used_storage': self.used_storage,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<User id={self.id}>'


