import { component$, useSignal } from "@builder.io/qwik";
import { useNavigate } from "@builder.io/qwik-city";
import { login } from "~/lib/api";

export default component$(() => {
  const nav = useNavigate();
  const username = useSignal("");
  const password = useSignal("");
  const error = useSignal("");
  const loading = useSignal(false);

  return (
    <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-indigo-950 via-indigo-900 to-slate-900">
      <div class="w-full max-w-sm mx-4">
        <div class="bg-white rounded-2xl shadow-2xl p-8">
          <div class="text-center mb-8">
            <div class="text-3xl mb-2">◆</div>
            <h1 class="text-2xl font-bold text-slate-800">Stora</h1>
            <p class="text-sm text-slate-500 mt-1">存你所存，想你所想</p>
          </div>

          {error.value && (
            <div class="mb-4 px-4 py-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded-lg">
              {error.value}
            </div>
          )}

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1">用户名</label>
              <input
                type="text"
                bind:value={username}
                onKeyDown$={(e: any) => {
                  if (e.key === "Enter") {
                    document.getElementById("login-btn")?.click();
                  }
                }}
                class="w-full px-4 py-2.5 border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="输入用户名"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1">密码</label>
              <input
                type="password"
                bind:value={password}
                onKeyDown$={(e: any) => {
                  if (e.key === "Enter") {
                    document.getElementById("login-btn")?.click();
                  }
                }}
                class="w-full px-4 py-2.5 border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="输入密码"
              />
            </div>
            <button
              id="login-btn"
              onClick$={async () => {
                if (!username.value || !password.value) {
                  error.value = "请输入用户名和密码";
                  return;
                }
                error.value = "";
                loading.value = true;
                try {
                  await login(username.value, password.value);
                  nav("/drive");
                } catch (e: any) {
                  error.value = e.message || "登录失败";
                } finally {
                  loading.value = false;
                }
              }}
              disabled={loading.value}
              class="w-full py-2.5 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 disabled:opacity-50 transition-colors"
            >
              {loading.value ? "登录中..." : "登录"}
            </button>
          </div>

          <div class="mt-6 text-center text-sm text-slate-500">
            还没有账号？
            <a href="/register" class="text-indigo-600 hover:text-indigo-800 font-medium ml-1">注册</a>
          </div>
        </div>
      </div>
    </div>
  );
});
