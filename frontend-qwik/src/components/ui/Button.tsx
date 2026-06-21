/**
 * Stora Button — enterprise button with variants
 */
import { component$, Slot } from "@builder.io/qwik";
import { Icon } from "./Icon";

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md" | "lg";

export const Button = component$<{
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  disabled?: boolean;
  class?: string;
  onClick$?: () => void;
  type?: "button" | "submit";
}>(({ variant = "primary", size = "md", loading, disabled, class: cls = "", onClick$, type = "button" }) => {
  const base = "inline-flex items-center justify-center gap-2 font-medium rounded-lg transition-all duration-150 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed active:scale-[0.97]";
  const sizes: Record<Size, string> = { sm: "px-3 py-1.5 text-xs", md: "px-4 py-2 text-sm", lg: "px-6 py-3 text-base" };
  const variants: Record<Variant, string> = {
    primary: "bg-indigo-600 text-white hover:bg-indigo-700 focus:ring-indigo-500 shadow-sm",
    secondary: "bg-white text-slate-700 border border-slate-300 hover:bg-slate-50 focus:ring-indigo-500",
    ghost: "text-slate-600 hover:bg-slate-100 focus:ring-slate-400",
    danger: "bg-red-600 text-white hover:bg-red-700 focus:ring-red-500 shadow-sm",
  };
  return (
    <button type={type} disabled={disabled || loading} onClick$={onClick$}
      class={`${base} ${sizes[size]} ${variants[variant]} ${cls}`}>
      {loading && <Icon name="spinner" class="animate-spin" size={size === "sm" ? 14 : size === "lg" ? 20 : 16} />}
      <Slot />
    </button>
  );
});

export const Input = component$<{
  value?: string;
  onInput$?: (e: Event) => void;
  placeholder?: string;
  label?: string;
  error?: string;
  type?: string;
  class?: string;
  prefix?: any;
}>(({ value, onInput$, placeholder, label, error, type = "text", class: cls = "", prefix }) => (
  <div class={cls}>
    {label && <label class="block text-sm font-medium text-slate-700 mb-1.5">{label}</label>}
    <div class="relative">
      {prefix && <div class="absolute inset-y-0 left-0 flex items-center pl-3 text-slate-400 pointer-events-none">{prefix}</div>}
      <input
        type={type}
        value={value}
        onInput$={onInput$}
        placeholder={placeholder}
        class={`w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900
          placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent
          disabled:opacity-50 disabled:cursor-not-allowed
          ${prefix ? "pl-10" : ""} ${error ? "border-red-300 focus:ring-red-500" : ""}`}
      />
    </div>
    {error && <p class="mt-1 text-xs text-red-600">{error}</p>}
  </div>
));

export const Skeleton = component$<{ class?: string }>(({ class: cls = "" }) => (
  <div class={`animate-pulse rounded-md bg-slate-200 ${cls}`} />
));

export const Badge = component$<{ variant?: "default" | "success" | "warning" | "danger"; class?: string }>(
  ({ variant = "default", class: cls = "" }) => {
    const variants = {
      default: "bg-slate-100 text-slate-700",
      success: "bg-green-50 text-green-700",
      warning: "bg-amber-50 text-amber-700",
      danger: "bg-red-50 text-red-700",
    };
    return <span class={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${variants[variant]} ${cls}`}><Slot /></span>;
  }
);
