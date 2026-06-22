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
    <div class="min-h-screen flex items-center justify-center bg-stora-foreground">
      <div class="w-full max-w-sm mx-4">
        <div class="bg-stora-card border border-stora-border p-8">
          <div class="text-center mb-8">
            <div class="w-10 h-10 bg-stora-primary flex items-center justify-center text-white text-lg font-bold mx-auto mb-4">S</div>
            <h1 class="text-2xl font-bold text-stora-foreground">注册 Stora</h1>
            <p class="text-sm text-stora-muted-foreground mt-1">创建你的个人存储空间</p>
          </div>

          {error.value && (
            <div class="mb-4 px-4 py-3 bg-red-50 border border-stora-border text-stora-destructive text-sm">
              {error.value}
            </div>
          )}

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-stora-foreground mb-1">用户名</label>
              <input type="text" bind:value={username}
                class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                placeholder="3-150 位字母/数字" />
            </div>
            <div>
              <label class="block text-sm font-medium text-stora-foreground mb-1">邮箱</label>
              <input type="email" bind:value={email}
                class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                placeholder="your@email.com" />
            </div>
            <div>
              <label class="block text-sm font-medium text-stora-foreground mb-1">密码</label>
              <input type="password" bind:value={password}
                class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                placeholder="至少 6 位" />
            </div>
            <div>
              <label class="block text-sm font-medium text-stora-foreground mb-1">确认密码</label>
              <input type="password" bind:value={confirm}
                onKeyDown$={(e: any) => { if (e.key === "Enter") document.getElementById("reg-btn")?.click(); }}
                class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                placeholder="再次输入密码" />
            </div>
            <button id="reg-btn" onClick$={async () => {
              if (!username.value || !email.value || !password.value) { error.value = "请填写所有必填字段"; return; }
              if (password.value.length < 6) { error.value = "密码至少 6 位"; return; }
              if (password.value !== confirm.value) { error.value = "两次密码不一致"; return; }
              error.value = ""; loading.value = true;
              try { await register({ username: username.value, email: email.value, password: password.value, password_confirm: confirm.value }); nav("/login"); }
              catch (e: any) { error.value = e.message || "注册失败"; }
              finally { loading.value = false; }
            }} disabled={loading.value}
              class="w-full h-12 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] disabled:opacity-50">
              {loading.value ? "注册中..." : "注册"}
            </button>
          </div>

          <div class="mt-6 text-center text-sm text-stora-muted-foreground">
            已有账号？<a href="/login" class="text-stora-primary hover:text-[#1D4ED8] font-medium ml-1">登录</a>
          </div>
        </div>
      </div>
    </div>
  );
});
