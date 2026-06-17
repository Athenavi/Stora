'use client';

import React, {useState} from 'react';
import {useQuery, useMutation, useQueryClient} from '@tanstack/react-query';
import {AuthGuard} from '@/components/AuthGuard';
import {QueryProvider} from '@/components/QueryProvider';
import {AdminShell} from '@/components/admin/AdminShell';
import {apiClient} from '@/lib/api/base-client';
import {Code, Plus, Trash2, Eye, Copy, Check, X} from 'lucide-react';

const PLUGIN_SLUG = 'code-snippets';
const ACTION_URL = `/plugins/${PLUGIN_SLUG}/action`;

interface Snippet {
  id: number;
  title: string;
  code: string;
  language: string;
  description: string;
  tags: string[];
  visibility: string;
  user_id: number;
  view_count: number;
  embed_count: number;
  created_at: string;
}

interface SnippetForm {
  title: string;
  code: string;
  language: string;
  description: string;
  tags: string;
  visibility: 'public' | 'private' | 'unlisted';
}

const EMPTY_FORM: SnippetForm = {title: '', code: '', language: 'python', description: '', tags: '', visibility: 'public'};

const SUPPORTED_LANGUAGES = [
  'python', 'javascript', 'typescript', 'java', 'cpp', 'c',
  'html', 'css', 'sql', 'bash', 'json', 'yaml', 'markdown',
];

function useCurrentUser() {
  return useQuery({
    queryKey: ['current-user'],
    queryFn: async () => {
      const res = await apiClient.get('/users/me');
      return res.data || {};
    },
    staleTime: 5 * 60 * 1000,
  });
}

function SnippetEditor({snippet, onSave, onCancel}: {snippet: SnippetForm; onSave: (d: SnippetForm) => void; onCancel: () => void}) {
  const [form, setForm] = useState<SnippetForm>(snippet);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onCancel}>
      <div className="bg-white dark:bg-gray-900 rounded-2xl w-full max-w-3xl max-h-[90vh] overflow-y-auto m-4 shadow-2xl border border-gray-200 dark:border-gray-800" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-800">
          <h2 className="text-lg font-bold text-gray-900 dark:text-white">{snippet.title ? 'Edit Snippet' : 'New Snippet'}</h2>
          <button onClick={onCancel} className="p-2 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"><X className="w-5 h-5 text-gray-400"/></button>
        </div>
        <div className="p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Title</label>
            <input type="text" value={form.title} onChange={e => setForm({...form, title: e.target.value})}
                   className="w-full px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500"/>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Language</label>
              <select value={form.language} onChange={e => setForm({...form, language: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm">
                {SUPPORTED_LANGUAGES.map(l => <option key={l} value={l}>{l}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Visibility</label>
              <select value={form.visibility} onChange={e => setForm({...form, visibility: e.target.value as any})}
                      className="w-full px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm">
                <option value="public">Public</option>
                <option value="private">Private</option>
                <option value="unlisted">Unlisted</option>
              </select>
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Description</label>
            <input type="text" value={form.description} onChange={e => setForm({...form, description: e.target.value})}
                   className="w-full px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm"/>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Tags (comma-separated)</label>
            <input type="text" value={form.tags} onChange={e => setForm({...form, tags: e.target.value})}
                   placeholder="e.g. python, tutorial, beginner"
                   className="w-full px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm"/>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Code</label>
            <textarea value={form.code} onChange={e => setForm({...form, code: e.target.value})} rows={12}
                      className="w-full px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm font-mono"/>
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 p-6 border-t border-gray-200 dark:border-gray-800">
          <button onClick={onCancel} className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-xl transition-colors">Cancel</button>
          <button onClick={() => onSave(form)} className="px-6 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-xl transition-colors">{snippet.title ? 'Save' : 'Create'}</button>
        </div>
      </div>
    </div>
  );
}

function SnippetManager() {
  const queryClient = useQueryClient();
  const [showEditor, setShowEditor] = useState(false);
  const [editingSnippet, setEditingSnippet] = useState<SnippetForm>(EMPTY_FORM);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const {data: user} = useCurrentUser();
  const userId = (user as any)?.id || 0;

  const {data: snippets = [], isLoading} = useQuery({
    queryKey: ['code-snippets', userId],
    queryFn: () => {
      if (!userId) return [];
      return import('../api').then(m => m.SnippetService.list(userId));
    },
    enabled: !!userId,
  });

  const createMutation = useMutation({
    mutationFn: (form: SnippetForm) =>
      import('../api').then(m => m.SnippetService.create(
        {...form, tags: form.tags.split(',').map(t => t.trim()).filter(Boolean)}, userId)),
    onSuccess: () => { queryClient.invalidateQueries({queryKey: ['code-snippets']}); setShowEditor(false); setEditingSnippet(EMPTY_FORM); },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => import('../api').then(m => m.SnippetService.delete(id, userId)),
    onSuccess: () => queryClient.invalidateQueries({queryKey: ['code-snippets']}),
  });

  const copyEmbedCode = async (snippet: Snippet) => {
    const embed = `[snippet:${snippet.id}]`;
    try { await navigator.clipboard.writeText(embed); setCopiedId(snippet.id); setTimeout(() => setCopiedId(null), 2000); } catch {}
  };

  const filtered = snippets.filter((s: Snippet) =>
    !searchQuery || s.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
    s.tags?.some(t => t.toLowerCase().includes(searchQuery.toLowerCase())));

  return (
    <AdminShell title="Snippets" actions={
      <button onClick={() => { setEditingSnippet(EMPTY_FORM); setShowEditor(true); }}
              className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-xl transition-colors">
        <Plus className="w-4 h-4"/> New Snippet
      </button>
    }>
      <div className="mb-6">
        <input type="text" placeholder="Search snippets..." value={searchQuery} onChange={e => setSearchQuery(e.target.value)}
               className="w-full max-w-md px-4 py-2 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm"/>
      </div>
      {isLoading ? (
        <div className="space-y-3">{[1,2,3].map(i => <div key={i} className="h-20 bg-gray-100 dark:bg-gray-800 rounded-xl animate-pulse"/>)}</div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-20 text-gray-400">
          <Code className="w-12 h-12 mx-auto mb-4 opacity-50"/>
          <p className="text-lg font-medium text-gray-500 dark:text-gray-400 mb-1">No snippets yet</p>
          <p className="text-sm">Click "New Snippet" to create one</p>
        </div>
      ) : (
        <div className="space-y-3">
          {filtered.map((snippet: Snippet) => (
            <div key={snippet.id} className="p-4 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-xl hover:border-gray-300 dark:hover:border-gray-700 transition-colors">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                  <Code className="w-4 h-4 text-blue-500"/>
                  <h3 className="font-semibold text-gray-900 dark:text-white text-sm">{snippet.title}</h3>
                  <span className="text-xs px-2 py-0.5 bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400 rounded-full">{snippet.language}</span>
                  {snippet.visibility !== 'public' && <span className="text-xs px-2 py-0.5 bg-gray-100 dark:bg-gray-800 text-gray-500 rounded-full">{snippet.visibility}</span>}
                </div>
              </div>
              {snippet.description && <p className="text-sm text-gray-500 dark:text-gray-400 mb-2 line-clamp-1">{snippet.description}</p>}
              {snippet.tags && snippet.tags.length > 0 && (
                <div className="flex gap-1.5 mb-2 flex-wrap">
                  {snippet.tags.map((tag: string) => <span key={tag} className="text-xs px-2 py-0.5 bg-gray-100 dark:bg-gray-800 text-gray-500 rounded-full">{tag}</span>)}
                </div>
              )}
              <div className="flex items-center justify-between text-xs text-gray-400">
                <div className="flex items-center gap-3">
                  <span className="flex items-center gap-1"><Eye className="w-3 h-3"/>{snippet.view_count}</span>
                  <span className="flex items-center gap-1"><Code className="w-3 h-3"/>{snippet.embed_count}</span>
                  <span>{snippet.created_at ? new Date(snippet.created_at).toLocaleDateString() : ''}</span>
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => copyEmbedCode(snippet)} className="p-1.5 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors" title="Copy embed [snippet:N]">
                    {copiedId === snippet.id ? <Check className="w-4 h-4 text-green-500"/> : <Copy className="w-4 h-4"/>}
                  </button>
                  <button onClick={() => deleteMutation.mutate(snippet.id)} className="p-1.5 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors text-gray-400 hover:text-red-500" title="Delete">
                    <Trash2 className="w-4 h-4"/>
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
      {showEditor && (
        <SnippetEditor snippet={editingSnippet}
                       onSave={data => createMutation.mutate(data)}
                       onCancel={() => { setShowEditor(false); setEditingSnippet(EMPTY_FORM); }}/>
      )}
    </AdminShell>
  );
}

export default function AdminCodeSnippets() {
  return <AuthGuard><QueryProvider><SnippetManager/></QueryProvider></AuthGuard>;
}
