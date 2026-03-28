import { z } from "zod";
import {
  allocationPolicies,
  emptyAccountBehaviors,
  manualUnavailableActions,
  missActions,
  stickyLeaseModes,
} from "./constants";
import { parseHeaderLines, parseLinesToList } from "./formParsers";
import type { EnvConfig } from "../systemConfig/types";
import type {
  Platform,
  PlatformAllocationPolicy,
  PlatformCreateInput,
  PlatformEmptyAccountBehavior,
  PlatformMissAction,
  PlatformUpdateInput,
} from "./types";

const platformNameForbiddenChars = ".:|/\\@?#%~";
const platformNameForbiddenSpacing = " \t\r\n";
const platformNameReserved = "api";

function containsAny(source: string, chars: string): boolean {
  for (const ch of chars) {
    if (source.includes(ch)) {
      return true;
    }
  }
  return false;
}

export const platformNameRuleHint = "平台名不能包含 .:|/\\@?#%~、空格、Tab、换行、回车，也不能为保留字。";

export const platformFormSchema = z.object({
  name: z.string().trim()
    .min(1, "平台名称不能为空")
    .refine((value) => !containsAny(value, platformNameForbiddenChars), {
      message: "平台名称不能包含字符 .:|/\\@?#%~",
    })
    .refine((value) => !containsAny(value, platformNameForbiddenSpacing), {
      message: "平台名称不能包含空格、Tab、换行、回车",
    })
    .refine((value) => value.toLowerCase() !== platformNameReserved, {
      message: "平台名称不能为保留字",
    }),
  sticky_ttl: z.string().optional(),
  sticky_lease_mode: z.enum(stickyLeaseModes),
  manual_unavailable_action: z.enum(manualUnavailableActions),
  manual_unavailable_grace: z.string().optional(),
  regex_filters_text: z.string().optional(),
  regex_filter_invert: z.boolean(),
  region_filters_text: z.string().optional(),
  region_filter_invert: z.boolean(),
  reverse_proxy_miss_action: z.enum(missActions),
  reverse_proxy_empty_account_behavior: z.enum(emptyAccountBehaviors),
  reverse_proxy_fixed_account_header: z.string().optional(),
  allocation_policy: z.enum(allocationPolicies),
}).superRefine((value, ctx) => {
  if (
    value.reverse_proxy_empty_account_behavior === "FIXED_HEADER" &&
    parseHeaderLines(value.reverse_proxy_fixed_account_header).length === 0
  ) {
    ctx.addIssue({
      code: "custom",
      path: ["reverse_proxy_fixed_account_header"],
      message: "用于提取 Account 的 Headers 不能为空",
    });
  }
  if (value.sticky_lease_mode === "MANUAL" && !value.manual_unavailable_grace?.trim()) {
    ctx.addIssue({
      code: "custom",
      path: ["manual_unavailable_grace"],
      message: "长租约模式下不可用观察期不能为空",
    });
  }
});

export type PlatformFormValues = z.infer<typeof platformFormSchema>;

export const defaultPlatformFormValues: PlatformFormValues = {
  name: "",
  sticky_ttl: "",
  sticky_lease_mode: "TTL",
  manual_unavailable_action: "HOLD",
  manual_unavailable_grace: "0s",
  regex_filters_text: "",
  regex_filter_invert: false,
  region_filters_text: "",
  region_filter_invert: false,
  reverse_proxy_miss_action: "TREAT_AS_EMPTY",
  reverse_proxy_empty_account_behavior: "RANDOM",
  reverse_proxy_fixed_account_header: "Authorization",
  allocation_policy: "BALANCED",
};

export function platformToFormValues(platform: Platform): PlatformFormValues {
  const regexFilters = Array.isArray(platform.regex_filters) ? platform.regex_filters : [];
  const regionFilters = Array.isArray(platform.region_filters) ? platform.region_filters : [];

  return {
    name: platform.name,
    sticky_ttl: platform.sticky_ttl,
    sticky_lease_mode: platform.sticky_lease_mode,
    manual_unavailable_action: platform.manual_unavailable_action,
    manual_unavailable_grace: platform.manual_unavailable_grace,
    regex_filters_text: regexFilters.join("\n"),
    regex_filter_invert: platform.regex_filter_invert,
    region_filters_text: regionFilters.join("\n"),
    region_filter_invert: platform.region_filter_invert,
    reverse_proxy_miss_action: platform.reverse_proxy_miss_action,
    reverse_proxy_empty_account_behavior: platform.reverse_proxy_empty_account_behavior,
    reverse_proxy_fixed_account_header: platform.reverse_proxy_fixed_account_header,
    allocation_policy: platform.allocation_policy,
  };
}

function toPlatformPayloadBase(values: PlatformFormValues) {
  return {
    name: values.name.trim(),
    sticky_ttl: values.sticky_ttl?.trim() || "",
    sticky_lease_mode: values.sticky_lease_mode,
    manual_unavailable_action: values.manual_unavailable_action,
    manual_unavailable_grace: values.manual_unavailable_grace?.trim() || "0s",
    regex_filters: parseLinesToList(values.regex_filters_text),
    regex_filter_invert: values.regex_filter_invert,
    region_filters: parseLinesToList(values.region_filters_text, (value) => value.toLowerCase()),
    region_filter_invert: values.region_filter_invert,
    reverse_proxy_miss_action: values.reverse_proxy_miss_action,
    reverse_proxy_empty_account_behavior: values.reverse_proxy_empty_account_behavior,
    reverse_proxy_fixed_account_header: parseHeaderLines(values.reverse_proxy_fixed_account_header).join("\n"),
    allocation_policy: values.allocation_policy,
  };
}

function areStringSlicesEqual(left: string[], right: string[]): boolean {
  if (left.length !== right.length) {
    return false;
  }
  for (let i = 0; i < left.length; i += 1) {
    if (left[i] !== right[i]) {
      return false;
    }
  }
  return true;
}

function asMissAction(value: string): PlatformMissAction {
  return value === "REJECT" ? "REJECT" : "TREAT_AS_EMPTY";
}

function asEmptyAccountBehavior(value: string): PlatformEmptyAccountBehavior {
  if (value === "FIXED_HEADER" || value === "ACCOUNT_HEADER_RULE") {
    return value;
  }
  return "RANDOM";
}

function asAllocationPolicy(value: string): PlatformAllocationPolicy {
  if (value === "PREFER_LOW_LATENCY" || value === "PREFER_IDLE_IP") {
    return value;
  }
  return "BALANCED";
}

export function platformFormValuesFromEnvConfig(env: EnvConfig): PlatformFormValues {
  return {
    name: "",
    sticky_ttl: env.default_platform_sticky_ttl,
    sticky_lease_mode: "TTL",
    manual_unavailable_action: "HOLD",
    manual_unavailable_grace: "0s",
    regex_filters_text: (env.default_platform_regex_filters ?? []).join("\n"),
    regex_filter_invert: env.default_platform_regex_filter_invert,
    region_filters_text: (env.default_platform_region_filters ?? []).join("\n"),
    region_filter_invert: env.default_platform_region_filter_invert,
    reverse_proxy_miss_action: asMissAction(env.default_platform_reverse_proxy_miss_action),
    reverse_proxy_empty_account_behavior: asEmptyAccountBehavior(env.default_platform_reverse_proxy_empty_account_behavior),
    reverse_proxy_fixed_account_header: env.default_platform_reverse_proxy_fixed_account_header,
    allocation_policy: asAllocationPolicy(env.default_platform_allocation_policy),
  };
}

export function toPlatformCreateInput(
  values: PlatformFormValues,
  defaults: PlatformFormValues = defaultPlatformFormValues,
): PlatformCreateInput {
  const current = toPlatformPayloadBase(values);
  const baseline = toPlatformPayloadBase(defaults);
  const payload: PlatformCreateInput = { name: current.name };

  if (current.sticky_ttl !== baseline.sticky_ttl) {
    payload.sticky_ttl = current.sticky_ttl || undefined;
  }
  if (current.sticky_lease_mode !== baseline.sticky_lease_mode) {
    payload.sticky_lease_mode = current.sticky_lease_mode;
  }
  if (current.manual_unavailable_action !== baseline.manual_unavailable_action) {
    payload.manual_unavailable_action = current.manual_unavailable_action;
  }
  if (current.manual_unavailable_grace !== baseline.manual_unavailable_grace) {
    payload.manual_unavailable_grace = current.manual_unavailable_grace;
  }
  if (!areStringSlicesEqual(current.regex_filters, baseline.regex_filters)) {
    payload.regex_filters = current.regex_filters;
  }
  if (current.regex_filter_invert !== baseline.regex_filter_invert) {
    payload.regex_filter_invert = current.regex_filter_invert;
  }
  if (!areStringSlicesEqual(current.region_filters, baseline.region_filters)) {
    payload.region_filters = current.region_filters;
  }
  if (current.region_filter_invert !== baseline.region_filter_invert) {
    payload.region_filter_invert = current.region_filter_invert;
  }
  if (current.reverse_proxy_miss_action !== baseline.reverse_proxy_miss_action) {
    payload.reverse_proxy_miss_action = current.reverse_proxy_miss_action;
  }
  if (current.reverse_proxy_empty_account_behavior !== baseline.reverse_proxy_empty_account_behavior) {
    payload.reverse_proxy_empty_account_behavior = current.reverse_proxy_empty_account_behavior;
  }
  if (current.reverse_proxy_fixed_account_header !== baseline.reverse_proxy_fixed_account_header) {
    payload.reverse_proxy_fixed_account_header = current.reverse_proxy_fixed_account_header;
  }
  if (current.allocation_policy !== baseline.allocation_policy) {
    payload.allocation_policy = current.allocation_policy;
  }

  return payload;
}

export function toPlatformUpdateInput(values: PlatformFormValues): PlatformUpdateInput {
  const payload = toPlatformPayloadBase(values);
  return {
    ...payload,
    sticky_ttl: payload.sticky_ttl,
  };
}
