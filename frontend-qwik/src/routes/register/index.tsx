import { component$, useSignal } from "@builder.io/qwik";
import { useNavigate } from "@builder.io/qwik-city";
import { register } from "~/lib/api";

export default component$(() => {
  const nav = useNavigate();
  const username = useSignal("");
  const email = useSignal("");
  const password = useSignal("");
  const confirm = useSignal("");
  const error = useSignal("");
  const loading = useSignal(false);

  return (
    <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-indigo-950 via-indigo-900 to-slate-900">
      <div class="w-full max-w-sm mx-4">
        <div class="bg-white rounded-2xl shadow-2xl p-8">
          <div class="text-center mb-8">
            <div class="text-3xl mb-2">◆</div>
            <h1 class="text-2xl font-bold text-slate-800">注册 Stora</h1>
            <p class="text-sm text-slate-500 mt-1">创建你的个人存储空间</p>
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
                class="w-full px-4 py-2.5 border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="3-150 位字母/数字"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1">邮箱</label>
              <input
                type="email"
                bind:value={email}
                class="w-full px-4 py-2.5 border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="your@email.com"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1">密码</label>
              <input
                type="password"
                bind:value={password}
                class="w-full px-4 py-2.5 border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="至少 6 位"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-slate-700 mb-1">确认密码</label>
              <input
                type="password"
                bind:value={confirm}
                onKeyDown$={(e: any) => {
                  if (e.key === "Enter") document.getElementById("reg-btn")?.click();
                }}
                class="w-full px-4 py-2.5 border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                placeholder="再次输入密码"
              />
            </div>
            <button
              id="reg-btn"
              onClick$={async () => {
                if (!username.value || !email.value || !password.value) {
                  error.value = "请填写所有必填字段";
                  return;
                }
                if (password.value.length < 6) {
                  error.value = "密码至少 6 位";
                  return;
                }
                if (password.value !== confirm.value) {
                  error.value = "两次密码不一致";
                  return;
                }
                error.value = "";
                loading.value = true;
                try {
                  await register({
                    username: username.value,
                    email: email.value,
                    password: password.value,
                    password_confirm: confirm.value,
                  });
                  nav("/login");
                } catch (e: any) {
                  error.value = e.message || "注册失败";
                } finally {
                  loading.value = false;
                }
              }}
              disabled={loading.value}
              class="w-full py-2.5 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 disabled:opacity-50 transition-colors"
            >
              {loading.value ? "注册中..." : "注册"}
            </button>
          </div>

          <div class="mt-6 text-center text-sm text-slate-500">
            已有账号？
            <a href="/login" class="text-indigo-600 hover:text-indigo-800 font-medium ml-1">登录</a>
          </div>
        </div>
      </div>
    </div>
  );
});
