/**
 * Stora Admin Page — user management, storage plans, audit logs
 */
import { component$, useSignal, $ } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { Button, Skeleton } from "~/components/ui/Button";
import { api } from "~/lib/api";

const tabs = [
  { id: "overview", label: "概览", icon: "eye" },
  { id: "users", label: "用户管理", icon: "user" },
  { id: "plans", label: "存储套餐", icon: "setting" },
  { id: "audit", label: "审计日志", icon: "search" },
];

export default component$(() => {
  const activeTab = useSignal("overview");

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">管理面板</h1>
      </div>

      {/* Tabs */}
      <div class="flex gap-1 px-6 py-2 border-b border-slate-200 bg-white/80 shrink-0 overflow-x-auto">
        {tabs.map(t => (
          <button key={t.id} onClick$={() => activeTab.value = t.id}
            class={`px-4 py-2 text-sm font-medium rounded-lg transition-colors whitespace-nowrap
              ${activeTab.value === t.id ? "bg-indigo-50 text-indigo-700" : "text-slate-500 hover:text-slate-700 hover:bg-slate-50"}`}>
            {t.label}
          </button>
        ))}
      </div>

      <div class="flex-1 overflow-auto p-6">
        {activeTab.value === "overview" && <OverviewTab />}
        {activeTab.value === "users" && <UsersTab />}
        {activeTab.value === "plans" && <PlansTab />}
        {activeTab.value === "audit" && <AuditTab />}
      </div>
    </div>
  );
});

function OverviewTab() {
  return (
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="bg-white rounded-xl border border-slate-200 p-6"><div class="text-2xl font-bold text-slate-900">—</div><div class="text-sm text-slate-500 mt-1">总用户</div></div>
      <div class="bg-white rounded-xl border border-slate-200 p-6"><div class="text-2xl font-bold text-slate-900">—</div><div class="text-sm text-slate-500 mt-1">总文件</div></div>
      <div class="bg-white rounded-xl border border-slate-200 p-6"><div class="text-2xl font-bold text-slate-900">—</div><div class="text-sm text-slate-500 mt-1">存储使用</div></div>
    </div>
  );
}

function UsersTab() {
  return <div class="text-slate-400 text-sm">用户管理功能</div>;
}

function PlansTab() {
  return <div class="text-slate-400 text-sm">存储套餐管理功能</div>;
}

function AuditTab() {
  return <div class="text-slate-400 text-sm">审计日志功能</div>;
}
