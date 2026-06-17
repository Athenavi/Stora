import { component$ } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";

export const useRedirect = routeLoader$(() => {
  return { redirect: "/drive" };
});

export default component$(() => {
  return <div class="flex items-center justify-center min-h-screen text-slate-400">重定向中...</div>;
});
