import { component$, useSignal, $ } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { api, createServerApi } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";

interface Team {
  id: number;
  name: string;
  description: string;
  owner_id: number;
  member_count: number;
  created_at: string;
}

interface Member {
  id: number;
  user_id: number;
  username: string;
  email: string;
  role: string;
  created_at: string;
}

export const useTeams = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  return await srv.get<Team[]>("/teams").catch(() => [] as Team[]);
});

export default component$(() => {
  const teamsLoader = useTeams();
  const teams = useSignal<Team[]>(teamsLoader.value || []);
  const loading = useSignal(false);
  const showCreate = useSignal(false);
  const newName = useSignal("");
  const newDesc = useSignal("");
  const expandedTeam = useSignal<number | null>(null);
  const members = useSignal<Member[]>([]);
  const memberLoading = useSignal(false);
  const searchQ = useSignal("");
  const searchResults = useSignal<{ id: number; username: string; email: string }[]>([]);

  const refresh = $(async () => {
    try {
      teams.value = (await api.get<Team[]>("/teams")) || [];
    } catch {}
  });

  const doCreate = $(async () => {
    if (!newName.value.trim()) return;
    loading.value = true;
    try {
      await api.post("/teams", { name: newName.value, description: newDesc.value });
      showCreate.value = false;
      newName.value = "";
      newDesc.value = "";
      await refresh();
    } catch (e: any) {
      alert(e.message || "创建失败");
    }
    loading.value = false;
  });

  const doDelete = $(async (id: number) => {
    if (!confirm("确认删除该团队？")) return;
    try {
      await api.delete(`/teams/${id}`);
      if (expandedTeam.value === id) expandedTeam.value = null;
      teams.value = teams.value.filter(t => t.id !== id);
    } catch {}
  });

  const loadMembers = $(async (teamId: number) => {
    if (expandedTeam.value === teamId) {
      expandedTeam.value = null;
      return;
    }
    expandedTeam.value = teamId;
    memberLoading.value = true;
    try {
      members.value = (await api.get<Member[]>(`/teams/${teamId}/members`)) || [];
    } catch { members.value = []; }
    memberLoading.value = false;
  });

  const doSearch = $(async (q: string) => {
    searchQ.value = q;
    if (!q.trim()) { searchResults.value = []; return; }
    try {
      searchResults.value = (await api.get<{ id: number; username: string; email: string }[]>(`/users/search?q=${encodeURIComponent(q)}`)) || [];
    } catch { searchResults.value = []; }
  });

  const addMember = $(async (teamId: number, userId: number) => {
    try {
      await api.post(`/teams/${teamId}/members`, { user_id: userId });
      searchQ.value = "";
      searchResults.value = [];
      members.value = (await api.get<Member[]>(`/teams/${teamId}/members`)) || [];
    } catch (e: any) {
      alert(e.message || "添加成员失败");
    }
  });

  const removeMember = $(async (teamId: number, memberId: number) => {
    if (!confirm("确认移除此成员？")) return;
    try {
      await api.delete(`/teams/${teamId}/members/${memberId}`);
      members.value = members.value.filter(m => m.id !== memberId);
      // Update member count in team list
      teams.value = teams.value.map(t =>
        t.id === teamId ? { ...t, member_count: Math.max(0, t.member_count - 1) } : t
      );
    } catch {}
  });

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <div class="flex items-center justify-between">
          <div>
            <h1 class="text-[28px] font-bold text-stora-foreground">团队</h1>
            <p class="text-sm text-stora-muted-foreground mt-1">管理团队和成员权限</p>
          </div>
          <button onClick$={() => showCreate.value = !showCreate.value}
            class="flex items-center gap-2 h-10 px-4 rounded-lg bg-stora-primary text-white text-sm font-medium hover:opacity-90 transition-opacity">
            <Icon name="plus" size={18} />
            新建团队
          </button>
        </div>
      </div>

      {/* Create form */}
      {showCreate.value && (
        <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
          <div class="flex items-end gap-3">
            <div class="flex-1">
              <label class="text-xs text-stora-muted-foreground mb-1 block">团队名称</label>
              <input bind:value={newName} placeholder="输入团队名称"
                class="w-full h-10 px-3 rounded-lg border border-stora-border bg-stora-background text-sm text-stora-foreground outline-none focus:border-stora-primary" />
            </div>
            <div class="flex-1">
              <label class="text-xs text-stora-muted-foreground mb-1 block">描述（可选）</label>
              <input bind:value={newDesc} placeholder="团队描述"
                class="w-full h-10 px-3 rounded-lg border border-stora-border bg-stora-background text-sm text-stora-foreground outline-none focus:border-stora-primary" />
            </div>
            <button onClick$={doCreate} disabled={loading.value}
              class="h-10 px-4 rounded-lg bg-stora-primary text-white text-sm font-medium hover:opacity-90 transition-opacity disabled:opacity-50">
              创建
            </button>
          </div>
        </div>
      )}

      {/* Team list */}
      <div class="flex-1 overflow-auto p-6">
        {teams.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
            <Icon name="share" size={48} />
            <h3 class="text-lg font-medium mt-4 text-stora-foreground">还没有团队</h3>
            <p class="text-sm mt-1">创建一个团队来共享文件夹和协作</p>
            <button onClick$={() => showCreate.value = true}
              class="mt-4 h-10 px-4 rounded-lg bg-stora-primary text-white text-sm font-medium hover:opacity-90 transition-opacity">
              新建团队
            </button>
          </div>
        ) : (
          <div class="space-y-3">
            {teams.value.map(team => (
              <div key={team.id} class="bg-stora-card rounded-lg border border-stora-border overflow-hidden">
                {/* Team row */}
                <div class="flex items-center gap-4 px-6 py-4 cursor-pointer hover:bg-stora-muted transition-colors"
                  onClick$={() => loadMembers(team.id)}>
                  <div class="w-10 h-10 rounded-lg bg-stora-primary/10 flex items-center justify-center shrink-0">
                    <Icon name="share" size={20} class="text-stora-primary" />
                  </div>
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-medium text-stora-foreground truncate">{team.name}</h3>
                    <p class="text-xs text-stora-muted-foreground truncate">{team.description || "无描述"}</p>
                  </div>
                  <div class="flex items-center gap-4 text-xs text-stora-muted-foreground">
                    <span>{team.member_count} 人</span>
                    <Icon name="chevronUp" size={16} class={`transition-transform ${expandedTeam.value === team.id ? "" : "rotate-180"}`} />
                  </div>
                  <button onClick$={(e: any) => { e.stopPropagation(); doDelete(team.id); }}
                    class="p-2 rounded-lg hover:bg-red-50 text-stora-muted-foreground hover:text-red-500 transition-colors">
                    <Icon name="trash" size={16} />
                  </button>
                </div>

                {/* Expanded member area */}
                {expandedTeam.value === team.id && (
                  <div class="border-t border-stora-border px-6 py-4 bg-stora-background/50">
                    {memberLoading.value ? (
                      <p class="text-sm text-stora-muted-foreground">加载中...</p>
                    ) : (
                      <>
                        {/* Add member */}
                        <div class="flex items-center gap-2 mb-4">
                          <div class="relative flex-1">
                            <input
                              bind:value={searchQ}
                              onInput$={(e: any) => doSearch(e.target.value)}
                              placeholder="搜索用户..."
                              class="w-full h-9 px-3 rounded-lg border border-stora-border bg-stora-card text-sm text-stora-foreground outline-none focus:border-stora-primary" />
                            {searchResults.value.length > 0 && (
                              <div class="absolute top-full left-0 right-0 mt-1 bg-stora-card border border-stora-border rounded-lg shadow-lg z-10 max-h-40 overflow-auto">
                                {searchResults.value.map(u => (
                                  <button key={u.id} onClick$={() => addMember(team.id, u.id)}
                                    class="flex items-center gap-3 w-full px-3 py-2 text-sm text-stora-foreground hover:bg-stora-muted text-left">
                                    <span class="font-medium">{u.username}</span>
                                    <span class="text-stora-muted-foreground">{u.email}</span>
                                  </button>
                                ))}
                              </div>
                            )}
                          </div>
                        </div>

                        {/* Member list */}
                        {members.value.length === 0 ? (
                          <p class="text-sm text-stora-muted-foreground">暂无成员</p>
                        ) : (
                          <div class="space-y-2">
                            {members.value.map(m => (
                              <div key={m.id} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-stora-card transition-colors">
                                <div class="w-8 h-8 rounded-full bg-stora-accent flex items-center justify-center text-white text-xs font-bold shrink-0">
                                  {m.username.charAt(0).toUpperCase()}
                                </div>
                                <div class="flex-1 min-w-0">
                                  <p class="text-sm font-medium text-stora-foreground truncate">{m.username}</p>
                                  <p class="text-xs text-stora-muted-foreground truncate">{m.email}</p>
                                </div>
                                {team.owner_id !== m.user_id && (
                                  <button onClick$={() => removeMember(team.id, m.id)}
                                    class="p-1.5 rounded-lg hover:bg-red-50 text-stora-muted-foreground hover:text-red-500 transition-colors">
                                    <Icon name="close" size={14} />
                                  </button>
                                )}
                              </div>
                            ))}
                          </div>
                        )}
                      </>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
