'use client';

import React from 'react';
import {useQuery} from '@tanstack/react-query';
import {AuthGuard} from '@/components/AuthGuard';
import {QueryProvider} from '@/components/QueryProvider';
import {AdminShell} from '@/components/admin/AdminShell';
import {Heart, TrendingUp} from 'lucide-react';
import {ArticleLikesService} from '../api';

function PopularRanking() {
  const {data: popular = {data: []}, isLoading} = useQuery({
    queryKey: ['article-likes-popular'],
    queryFn: () => ArticleLikesService.popular(20),
  });

  return (
    <AdminShell title="Article Likes" actions={
      <span className="flex items-center gap-1 text-sm text-gray-500">
        <Heart className="w-4 h-4 text-red-400"/>
        Ranking by likes
      </span>
    }>
      {isLoading ? (
        <div className="space-y-3">
          {[1,2,3,4,5].map(i => (
            <div key={i} className="h-14 bg-gray-100 dark:bg-gray-800 rounded-xl animate-pulse"/>
          ))}
        </div>
      ) : !(popular as any).data?.length ? (
        <div className="text-center py-20 text-gray-400">
          <Heart className="w-12 h-12 mx-auto mb-4 opacity-50"/>
          <p className="text-lg font-medium text-gray-500 dark:text-gray-400 mb-1">No likes yet</p>
          <p className="text-sm">Likes will appear here once readers start liking articles</p>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-hidden">
          <div className="divide-y divide-gray-100 dark:divide-gray-800/50">
            {(popular as any).data.map((item: any, i: number) => (
              <div key={item.article_id} className="flex items-center gap-4 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-800/30 transition-colors">
                <span className={`w-7 h-7 flex items-center justify-center rounded-full text-xs font-bold ${
                  i === 0 ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400' :
                  i === 1 ? 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300' :
                  i === 2 ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400' :
                  'text-gray-400'
                }`}>{i + 1}</span>
                <span className="flex-1 text-sm text-gray-900 dark:text-white">Article #{item.article_id}</span>
                <div className="flex items-center gap-1.5 text-sm font-medium text-red-500">
                  <Heart className="w-3.5 h-3.5 fill-current"/>
                  <span>{item.likes}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </AdminShell>
  );
}

export default function AdminArticleLikes() {
  return <AuthGuard><QueryProvider><PopularRanking/></QueryProvider></AuthGuard>;
}
