/**
 * Stora Login — Enterprise login page
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { useNavigate, useLocation } from "@builder.io/qwik-city";
import { login, setToken } from "~/lib/api";
import { Button } from "~/components/ui/Button";

export default component$(() => {
  const nav = useNavigate();
  const loc = useLocation();

  // Handle OAuth token in URL
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const token = loc.url.searchParams.get("token");
    if (token) {
      setToken(token);
      nav("/drive");
    }
  });
  const username = useSignal("");
  const password = useSignal("");
  const error = useSignal("");
  const loading = useSignal(false);

  return (
    <div class="min-h-screen flex bg-slate-50">
      {/* Left brand panel */}
      <div class="hidden lg:flex w-1/2 bg-gradient-to-br from-slate-900 via-indigo-950 to-slate-900 p-16 flex-col justify-between">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-indigo-500 flex items-center justify-center text-white text-lg font-bold">S</div>
          <span class="text-white text-xl font-semibold">Stora</span>
        </div>
        <div>
          <blockquote class="text-white/80 text-2xl font-light leading-relaxed max-w-md">
            "存你所存，想你所想。"
          </blockquote>
          <p class="text-slate-400 text-sm mt-3">Enterprise Storage Platform</p>
        </div>
        <div class="text-slate-600 text-xs">
          © 2026 Stora. All rights reserved.
        </div>
      </div>

      {/* Right form panel */}
      <div class="flex-1 flex items-center justify-center p-8">
        <div class="w-full max-w-sm">
          <div class="text-center lg:hidden mb-8">
            <h1 class="text-2xl font-bold text-slate-900">Stora</h1>
            <p class="text-sm text-slate-500 mt-1">Enterprise Storage</p>
          </div>

          <h2 class="text-xl font-semibold text-slate-900 mb-1">登录</h2>
          <p class="text-sm text-slate-500 mb-8">欢迎回来，请登录你的账户</p>

          {error.value && (
            <div class="mb-6 px-4 py-3 bg-red-50 border border-red-100 text-red-700 text-sm rounded-lg flex items-center gap-2">
              <span>⚠</span>
              <span>{error.value}</span>
            </div>
          )}

          <div class="space-y-5">
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1.5">用户名</label>
              <input type="text" bind:value={username}
                class="w-full px-3 py-2.5 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent placeholder:text-slate-400"
                placeholder="输入用户名" />
            </div>
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1.5">密码</label>
              <input type="password" bind:value={password}
                class="w-full px-3 py-2.5 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent placeholder:text-slate-400"
                placeholder="输入密码" />
            </div>
            <Button onClick$={async () => {
              if (!username.value || !password.value) { error.value = "请输入用户名和密码"; return; }
              error.value = ""; loading.value = true;
              try { await login(username.value, password.value); nav("/drive"); }
              catch (e: any) { error.value = e.message || "登录失败"; }
              finally { loading.value = false; }
            }} loading={loading.value} class="w-full" size="lg">
              登录
            </Button>
          </div>

          <p class="mt-8 text-center text-sm text-slate-500">
            还没有账号？<a href="/register" class="text-indigo-600 hover:text-indigo-800 font-medium">注册</a>
          </p>

          <div class="mt-6 pt-6 border-t border-slate-200">
            <p class="text-xs text-center text-slate-400 mb-4">或使用第三方账号登录</p>
            <div class="flex gap-3">
              <a href="/api/v2/auth/oauth/github"
                class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 border border-slate-300 rounded-lg text-sm text-slate-700 hover:bg-slate-50 transition-colors">
                <span>GitHub</span>
              </a>
              <a href="/api/v2/auth/oauth/google"
                class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 border border-slate-300 rounded-lg text-sm text-slate-700 hover:bg-slate-50 transition-colors">
                <span>Google</span>
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
});
