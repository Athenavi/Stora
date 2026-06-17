'use client';

import React, {useState, useEffect, useCallback} from 'react';
import {ArticleLikesService} from './api';
import {Heart} from 'lucide-react';

interface Props {
  articleId: number;
  userId?: number;
  initialCount?: number;
  initialLiked?: boolean;
  className?: string;
}

export default function LikeButton({articleId, userId, initialCount = 0, initialLiked = false, className = ''}: Props) {
  const [liked, setLiked] = useState(initialLiked);
  const [count, setCount] = useState(initialCount);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (userId && !initialLiked && initialCount === 0) {
      ArticleLikesService.status(articleId, userId).then(r => {
        if ((r as any).success) {
          setLiked((r as any).liked);
          setCount((r as any).count || 0);
        }
      }).catch(() => {});
    }
  }, [articleId, userId]);

  const toggle = useCallback(async () => {
    if (!userId || busy) return;
    setBusy(true);
    try {
      if (liked) {
        const r = await ArticleLikesService.unlike(articleId, userId);
        if ((r as any).success) {
          setLiked(false);
          setCount((r as any).count || 0);
        }
      } else {
        const r = await ArticleLikesService.like(articleId, userId);
        if ((r as any).success) {
          setLiked(true);
          setCount((r as any).count || 0);
        }
      }
    } catch { /* ignore */ }
    setBusy(false);
  }, [articleId, userId, liked, busy]);

  if (!userId) return null;

  return (
    <button onClick={toggle} disabled={busy}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border transition-all text-sm font-medium ${
              liked
                ? 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-600 dark:text-red-400'
                : 'border-gray-200 dark:border-gray-700 text-gray-500 dark:text-gray-400 hover:border-red-200 hover:text-red-400'
            } disabled:opacity-50 ${className}`}>
      <Heart className={`w-4 h-4 ${liked ? 'fill-current' : ''}`}/>
      <span>{count > 0 ? count : (liked ? '已赞' : '点赞')}</span>
    </button>
  );
}
