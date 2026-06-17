'use client';
import React from 'react';
import {AdminShell} from '@/components/admin/AdminShell';
import {QueryProvider} from '@/components/QueryProvider';
import ThemeConfigPanel from '@/components/plugins/ThemeConfigPanel';

export default function ModernMinimalThemeConfig() {
    return (
        <QueryProvider>
            <AdminShell title="简约主题配置">
                <ThemeConfigPanel
                    pluginSlug="modern-minimal"
                    themeName="Modern Minimal"
                    themeDescription="现代简约风格 - 支持代码高亮和深色模式"
                />
            </AdminShell>
        </QueryProvider>
    );
}
