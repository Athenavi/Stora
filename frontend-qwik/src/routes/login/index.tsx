/**
 * Stora Login — Enterprise login page with password + email code tabs
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { useNavigate, useLocation } from "@builder.io/qwik-city";
import { login, setToken, loginWithCode, sendCode } from "~/lib/api";

export default component$(() => {
  const nav = useNavigate();
  const loc = useLocation();
  const loginTab = useSignal<"password" | "code">("password");

  // Handle OAuth token in URL
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const token = loc.url.searchParams.get("token");
    if (token) {
      setToken(token);
      nav("/drive");
    }
  });

  // Password login fields
  const username = useSignal("");
  const password = useSignal("");
  const error = useSignal("");
  const loading = useSignal(false);

  // Code login fields
  const codeEmail = useSignal("");
  const code = useSignal("");
  const codeSent = useSignal(false);
  const countdown = useSignal(0);
  const codeLoading = useSignal(false);

  const doPasswordLogin = $(async () => {
    if (!username.value || !password.value) { error.value = "请输入用户名和密码"; return; }
    error.value = ""; loading.value = true;
    try { await login(username.value, password.value); nav("/drive"); }
    catch (e: any) { error.value = e.message || "登录失败"; }
    finally { loading.value = false; }
  });

  const doSendCode = $(async () => {
    if (!codeEmail.value || !codeEmail.value.includes("@")) { error.value = "请输入有效邮箱"; return; }
    error.value = ""; codeLoading.value = true;
    try {
      await sendCode(codeEmail.value);
      codeSent.value = true;
      countdown.value = 60;
      const interval = setInterval(() => {
        countdown.value--;
        if (countdown.value <= 0) { clearInterval(interval); }
      }, 1000);
    } catch (e: any) { error.value = e.message || "发送失败"; }
    finally { codeLoading.value = false; }
  });

  const doCodeLogin = $(async () => {
    if (!code.value) { error.value = "请输入验证码"; return; }
    error.value = ""; loading.value = true;
    try {
      await loginWithCode(codeEmail.value, code.value);
      nav("/drive");
    } catch (e: any) { error.value = e.message || "登录失败"; }
    finally { loading.value = false; }
  });

  return (
    <div class="min-h-screen flex bg-stora-background">
      {/* Left brand panel */}
      <div class="hidden lg:flex w-1/2 bg-stora-foreground p-16 flex-col justify-between">
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
            <h1 class="text-2xl font-bold text-stora-foreground">Stora</h1>
            <p class="text-sm text-stora-muted-foreground mt-1">Enterprise Storage</p>
          </div>

          <h2 class="text-xl font-semibold text-stora-foreground mb-1">登录</h2>
          <p class="text-sm text-stora-muted-foreground mb-6">欢迎回来，请登录你的账户</p>

          {/* Login tab switcher */}
          <div class="flex gap-1 mb-6 bg-stora-muted p-0.5">
            <button onClick$={() => loginTab.value = "password"}
              class={`flex-1 py-2 text-sm font-medium ${loginTab.value === "password" ? "bg-white text-stora-foreground" : "text-stora-muted-foreground"}`}>密码登录</button>
            <button onClick$={() => loginTab.value = "code"}
              class={`flex-1 py-2 text-sm font-medium ${loginTab.value === "code" ? "bg-white text-stora-foreground" : "text-stora-muted-foreground"}`}>验证码登录</button>
          </div>

          {error.value && (
            <div class="mb-6 px-4 py-3 bg-red-50 border border-stora-border text-stora-destructive text-sm flex items-center gap-2">
              <span>{error.value}</span>
            </div>
          )}

          {loginTab.value === "password" ? (
            <div class="space-y-5">
              <div>
                <label class="block text-sm font-medium text-stora-foreground mb-1.5">用户名</label>
                <input type="text" bind:value={username}
                  onKeyDown$={(e: any) => { if (e.key === "Enter") doPasswordLogin(); }}
                  class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                  placeholder="输入用户名" />
              </div>
              <div>
                <label class="block text-sm font-medium text-stora-foreground mb-1.5">密码</label>
                <input type="password" bind:value={password}
                  onKeyDown$={(e: any) => { if (e.key === "Enter") doPasswordLogin(); }}
                  class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                  placeholder="输入密码" />
              </div>
              <button onClick$={doPasswordLogin} disabled={loading.value}
                class="w-full h-12 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] flex items-center justify-center gap-2">
                {loading.value ? "登录中..." : "登录"}
              </button>
            </div>
          ) : (
            <div class="space-y-5">
              <div>
                <label class="block text-sm font-medium text-stora-foreground mb-1.5">邮箱</label>
                <input type="email" bind:value={codeEmail}
                  class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                  placeholder="输入邮箱地址" disabled={codeSent.value} />
              </div>
              {!codeSent.value ? (
                <button onClick$={doSendCode} disabled={codeLoading.value}
                  class="w-full h-12 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8]">
                  {codeLoading.value ? "发送中..." : "获取验证码"}
                </button>
              ) : (
                <>
                  <div>
                    <label class="block text-sm font-medium text-stora-foreground mb-1.5">验证码</label>
                    <input type="text" bind:value={code}
                      onKeyDown$={(e: any) => { if (e.key === "Enter") doCodeLogin(); }}
                      class="w-full h-12 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary"
                      placeholder="输入6位验证码" />
                  </div>
                  <button onClick$={doCodeLogin} disabled={loading.value}
                    class="w-full h-12 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8]">
                    {loading.value ? "登录中..." : "登录 / 注册"}
                  </button>
                  <div class="flex items-center justify-between text-xs text-stora-muted-foreground">
                    <span>验证码已发送到 {codeEmail.value}</span>
                    {countdown.value > 0 ? (
                      <span>{countdown.value}s 后可重新发送</span>
                    ) : (
                      <button onClick$={() => { codeSent.value = false; }} class="text-stora-primary hover:text-[#1D4ED8]">重新发送</button>
                    )}
                  </div>
                </>
              )}
            </div>
          )}

          <p class="mt-8 text-center text-sm text-stora-muted-foreground">
            还没有账号？<a href="/register" class="text-stora-primary hover:text-[#1D4ED8] font-medium">注册</a>
          </p>

          <div class="mt-6 pt-6 border-t border-stora-border">
            <p class="text-xs text-center text-stora-nav-text mb-4">或使用第三方账号登录</p>
            <div class="flex gap-3">
              <a href="/api/v2/auth/oauth/github"
                class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 border border-stora-border text-sm text-stora-foreground hover:bg-stora-muted">
                <span>GitHub</span>
              </a>
              <a href="/api/v2/auth/oauth/google"
                class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 border border-stora-border text-sm text-stora-foreground hover:bg-stora-muted">
                <span>Google</span>
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
});
