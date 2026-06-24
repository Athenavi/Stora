/**
 * Stora Account Settings — 账号设置页面
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi } from "~/lib/api";
import { getProfile, updateProfile, changePassword, logout } from "~/lib/api/auth";
import { Icon } from "~/components/ui/Icon";

export const useUserProfile = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  return await srv.get<{ id: number; username: string; email: string; bio?: string; date_joined?: string; is_superuser: boolean }>("/auth/me").catch(() => null);
});

export default component$(() => {
  const profile = useUserProfile();
  const nav = useNavigate();
  const saving = useSignal(false);
  const saveMsg = useSignal<{ type: "success" | "error"; text: string } | null>(null);

  const username = useSignal(profile.value?.username || "");
  const email = useSignal(profile.value?.email || "");
  const bio = useSignal(profile.value?.bio || "");

  const pwCurrent = useSignal("");
  const pwNew = useSignal("");
  const pwConfirm = useSignal("");
  const changingPw = useSignal(false);
  const pwMsg = useSignal<{ type: "success" | "error"; text: string } | null>(null);

  const saveProfile = $(async () => {
    saving.value = true;
    saveMsg.value = null;
    try {
      await updateProfile({ username: username.value, email: email.value, bio: bio.value });
      saveMsg.value = { type: "success", text: "资料已更新" };
    } catch (e: any) {
      saveMsg.value = { type: "error", text: e.message || "更新失败" };
    }
    saving.value = false;
  });

  const doChangePassword = $(async () => {
    if (pwNew.value !== pwConfirm.value) {
      pwMsg.value = { type: "error", text: "两次密码输入不一致" };
      return;
    }
    if (pwNew.value.length < 6) {
      pwMsg.value = { type: "error", text: "新密码至少 6 位" };
      return;
    }
    changingPw.value = true;
    pwMsg.value = null;
    try {
      await changePassword({ current_password: pwCurrent.value, new_password: pwNew.value, new_password_confirm: pwConfirm.value });
      pwMsg.value = { type: "success", text: "密码已修改" };
      pwCurrent.value = "";
      pwNew.value = "";
      pwConfirm.value = "";
    } catch (e: any) {
      pwMsg.value = { type: "error", text: e.message || "修改失败" };
    }
    changingPw.value = false;
  });

  const doLogout = $(async () => {
    await logout();
    location.href = "/login";
  });

  return (
    <div class="flex-1 overflow-auto bg-stora-background">
      <div class="max-w-2xl mx-auto px-6 py-8 space-y-8">
        {/* Header */}
        <div>
          <h1 class="text-[28px] font-bold text-stora-foreground">账号设置</h1>
          <p class="text-sm text-stora-muted-foreground mt-1">管理你的个人信息和安全设置</p>
        </div>

        {/* Profile info card */}
        <div class="bg-white rounded-xl border border-stora-border p-6 space-y-5">
          <h2 class="text-lg font-semibold text-stora-foreground flex items-center gap-2">
            <Icon name="user" size={20} class="text-stora-primary" /> 个人信息
          </h2>

          <div>
            <label class="block text-sm font-medium text-stora-foreground mb-1.5">用户名</label>
            <input type="text" bind:value={username}
              class="w-full px-3 py-2 text-sm border border-stora-border rounded-lg focus:outline-none focus:ring-1 focus:ring-stora-primary" />
          </div>

          <div>
            <label class="block text-sm font-medium text-stora-foreground mb-1.5">邮箱</label>
            <input type="email" bind:value={email}
              class="w-full px-3 py-2 text-sm border border-stora-border rounded-lg focus:outline-none focus:ring-1 focus:ring-stora-primary" />
          </div>

          <div>
            <label class="block text-sm font-medium text-stora-foreground mb-1.5">个人简介</label>
            <textarea bind:value={bio} rows={3}
              class="w-full px-3 py-2 text-sm border border-stora-border rounded-lg resize-none focus:outline-none focus:ring-1 focus:ring-stora-primary" placeholder="介绍一下自己..." />
          </div>

          <div class="flex items-center gap-3">
            <button onClick$={saveProfile} disabled={saving.value}
              class="px-5 py-2 text-sm font-medium text-white bg-stora-primary rounded-lg hover:bg-[#1D4ED8] transition-colors disabled:opacity-50">
              {saving.value ? "保存中..." : "保存"}
            </button>
            {saveMsg.value && (
              <span class={`text-sm ${saveMsg.value.type === "success" ? "text-green-600" : "text-red-500"}`}>
                {saveMsg.value.text}
              </span>
            )}
          </div>
        </div>

        {/* Password card */}
        <div class="bg-white rounded-xl border border-stora-border p-6 space-y-5">
          <h2 class="text-lg font-semibold text-stora-foreground flex items-center gap-2">
            <Icon name="lock" size={20} class="text-stora-primary" /> 修改密码
          </h2>

          <div>
            <label class="block text-sm font-medium text-stora-foreground mb-1.5">当前密码</label>
            <input type="password" bind:value={pwCurrent}
              class="w-full px-3 py-2 text-sm border border-stora-border rounded-lg focus:outline-none focus:ring-1 focus:ring-stora-primary" />
          </div>

          <div>
            <label class="block text-sm font-medium text-stora-foreground mb-1.5">新密码</label>
            <input type="password" bind:value={pwNew}
              class="w-full px-3 py-2 text-sm border border-stora-border rounded-lg focus:outline-none focus:ring-1 focus:ring-stora-primary" />
          </div>

          <div>
            <label class="block text-sm font-medium text-stora-foreground mb-1.5">确认新密码</label>
            <input type="password" bind:value={pwConfirm}
              class="w-full px-3 py-2 text-sm border border-stora-border rounded-lg focus:outline-none focus:ring-1 focus:ring-stora-primary" />
          </div>

          <div class="flex items-center gap-3">
            <button onClick$={doChangePassword} disabled={changingPw.value}
              class="px-5 py-2 text-sm font-medium text-white bg-stora-primary rounded-lg hover:bg-[#1D4ED8] transition-colors disabled:opacity-50">
              {changingPw.value ? "修改中..." : "修改密码"}
            </button>
            {pwMsg.value && (
              <span class={`text-sm ${pwMsg.value.type === "success" ? "text-green-600" : "text-red-500"}`}>
                {pwMsg.value.text}
              </span>
            )}
          </div>
        </div>

        {/* Account info card */}
        {profile.value && (
          <div class="bg-white rounded-xl border border-stora-border p-6 space-y-3">
            <h2 class="text-lg font-semibold text-stora-foreground flex items-center gap-2">
              <Icon name="eye" size={20} class="text-stora-primary" /> 账户信息
            </h2>
            <div class="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span class="text-stora-muted-foreground">用户 ID</span>
                <p class="text-stora-foreground font-mono">{profile.value.id}</p>
              </div>
              <div>
                <span class="text-stora-muted-foreground">角色</span>
                <p class="text-stora-foreground">{profile.value.is_superuser ? "管理员" : "普通用户"}</p>
              </div>
              {profile.value.date_joined && (
                <div>
                  <span class="text-stora-muted-foreground">注册时间</span>
                  <p class="text-stora-foreground">{new Date(profile.value.date_joined).toLocaleDateString("zh-CN")}</p>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Logout */}
        <div class="flex justify-center pb-8">
          <button onClick$={doLogout}
            class="flex items-center gap-2 px-6 py-2.5 text-sm font-medium text-red-500 border border-red-200 rounded-lg hover:bg-red-50 transition-colors">
            <Icon name="close" size={16} /> 退出登录
          </button>
        </div>
      </div>
    </div>
  );
});
