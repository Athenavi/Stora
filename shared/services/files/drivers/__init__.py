"""Stora File Storage Drivers — factory pattern"""
from typing import Union

from shared.services.files.drivers.s3_driver import S3Driver


def get_driver(name: str = "local") -> Union["LocalDriver", S3Driver]:
    """获取存储驱动实例"""
    if name == "s3":
        from shared.services.files.drivers.s3_driver import get_s3_driver
        return get_s3_driver()
    # Local is default
    from shared.services.files.file_service import driver
    return driver
