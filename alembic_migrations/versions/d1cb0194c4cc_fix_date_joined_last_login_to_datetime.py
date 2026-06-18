"""fix_date_joined_last_login_to_datetime

Revision ID: d1cb0194c4cc
Revises: 1cf4c5945a49
Create Date: 2026-06-18 11:47:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision: str = 'd1cb0194c4cc'
down_revision: Union[str, None] = '1cf4c5945a49'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.alter_column('users', 'date_joined',
                    existing_type=sa.String(255),
                    type_=sa.DateTime(timezone=True),
                    existing_nullable=True,
                    postgresql_using='date_joined::timestamp with time zone')
    op.alter_column('users', 'last_login_at',
                    existing_type=sa.String(255),
                    type_=sa.DateTime(timezone=True),
                    existing_nullable=True,
                    postgresql_using='last_login_at::timestamp with time zone')


def downgrade() -> None:
    op.alter_column('users', 'date_joined',
                    existing_type=sa.DateTime(timezone=True),
                    type_=sa.String(255),
                    existing_nullable=True)
    op.alter_column('users', 'last_login_at',
                    existing_type=sa.DateTime(timezone=True),
                    type_=sa.String(255),
                    existing_nullable=True)
