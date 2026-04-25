import { apiRequest } from "../../lib/api-client";
import type { PageResponse, LeaseResponse, Platform, PlatformCreateInput, PlatformUpdateInput } from "./types";

const basePath = "/api/v1/platforms";

type ApiPlatform = Omit<
  Platform,
  "regex_filters" | "regex_filter_invert" | "region_filters" | "region_filter_invert" | "subscription_filters" | "subscription_filter_invert"
> & {
  regex_filters?: string[] | null;
  regex_filter_invert?: boolean | null;
  region_filters?: string[] | null;
  region_filter_invert?: boolean | null;
  subscription_filters?: string[] | null;
  subscription_filter_invert?: boolean | null;
  sticky_lease_mode?: Platform["sticky_lease_mode"] | null;
  manual_unavailable_action?: Platform["manual_unavailable_action"] | null;
  manual_unavailable_grace?: string | null;
  routable_node_count?: number | null;
  reverse_proxy_miss_action?: Platform["reverse_proxy_miss_action"] | null;
  reverse_proxy_empty_account_behavior?: Platform["reverse_proxy_empty_account_behavior"] | null;
  reverse_proxy_fixed_account_header?: string | null;
};

function parseMissAction(raw: ApiPlatform["reverse_proxy_miss_action"]): Platform["reverse_proxy_miss_action"] {
  if (raw === "TREAT_AS_EMPTY" || raw === "REJECT") {
    return raw;
  }
  throw new Error(`invalid reverse_proxy_miss_action: ${String(raw)}`);
}

function normalizePlatform(raw: ApiPlatform): Platform {
  return {
    ...raw,
    reverse_proxy_miss_action: parseMissAction(raw.reverse_proxy_miss_action),
    sticky_lease_mode: raw.sticky_lease_mode === "MANUAL" ? "MANUAL" : "TTL",
    manual_unavailable_action: raw.manual_unavailable_action === "AUTO_CLEAN" ? "AUTO_CLEAN" : "HOLD",
    manual_unavailable_grace: typeof raw.manual_unavailable_grace === "string" ? raw.manual_unavailable_grace : "0s",
    regex_filters: Array.isArray(raw.regex_filters) ? raw.regex_filters : [],
    regex_filter_invert: raw.regex_filter_invert === true,
    region_filters: Array.isArray(raw.region_filters) ? raw.region_filters : [],
    region_filter_invert: raw.region_filter_invert === true,
    subscription_filters: Array.isArray(raw.subscription_filters) ? raw.subscription_filters : [],
    subscription_filter_invert: raw.subscription_filter_invert === true,
    routable_node_count: typeof raw.routable_node_count === "number" ? raw.routable_node_count : 0,
    reverse_proxy_empty_account_behavior:
      raw.reverse_proxy_empty_account_behavior === "RANDOM" ||
      raw.reverse_proxy_empty_account_behavior === "FIXED_HEADER" ||
      raw.reverse_proxy_empty_account_behavior === "ACCOUNT_HEADER_RULE"
        ? raw.reverse_proxy_empty_account_behavior
        : "RANDOM",
    reverse_proxy_fixed_account_header:
      typeof raw.reverse_proxy_fixed_account_header === "string" ? raw.reverse_proxy_fixed_account_header : "",
  };
}

function normalizePlatformPage(raw: PageResponse<ApiPlatform>): PageResponse<Platform> {
  return {
    ...raw,
    items: raw.items.map(normalizePlatform),
  };
}

export type ListPlatformsPageInput = {
  limit?: number;
  offset?: number;
  keyword?: string;
};

export async function listPlatforms(input: ListPlatformsPageInput = {}): Promise<PageResponse<Platform>> {
  const query = new URLSearchParams({
    limit: String(input.limit ?? 50),
    offset: String(input.offset ?? 0),
    sort_by: "name",
    sort_order: "asc",
  });
  const keyword = input.keyword?.trim();
  if (keyword) {
    query.set("keyword", keyword);
  }

  const data = await apiRequest<PageResponse<ApiPlatform>>(`${basePath}?${query.toString()}`);
  return normalizePlatformPage(data);
}

export async function getPlatform(id: string): Promise<Platform> {
  const data = await apiRequest<ApiPlatform>(`${basePath}/${id}`);
  return normalizePlatform(data);
}

export async function createPlatform(input: PlatformCreateInput): Promise<Platform> {
  const data = await apiRequest<ApiPlatform>(basePath, {
    method: "POST",
    body: input,
  });
  return normalizePlatform(data);
}

export async function updatePlatform(id: string, input: PlatformUpdateInput): Promise<Platform> {
  const data = await apiRequest<ApiPlatform>(`${basePath}/${id}`, {
    method: "PATCH",
    body: input,
  });
  return normalizePlatform(data);
}

export async function deletePlatform(id: string): Promise<void> {
  await apiRequest<void>(`${basePath}/${id}`, {
    method: "DELETE",
  });
}

export async function resetPlatform(id: string): Promise<Platform> {
  const data = await apiRequest<ApiPlatform>(`${basePath}/${id}/actions/reset-to-default`, {
    method: "POST",
  });
  return normalizePlatform(data);
}

export async function rebuildPlatform(id: string): Promise<void> {
  await apiRequest<{ status: "ok" }>(`${basePath}/${id}/actions/rebuild-routable-view`, {
    method: "POST",
  });
}

export async function clearAllPlatformLeases(id: string): Promise<void> {
  await apiRequest<void>(`${basePath}/${id}/leases`, {
    method: "DELETE",
  });
}

export type ListLeasesInput = {
  limit?: number;
  offset?: number;
  account?: string;
  fuzzy?: boolean;
  sort_by?: "account" | "expiry" | "last_accessed";
};

export async function listLeases(platformId: string, input: ListLeasesInput = {}): Promise<PageResponse<LeaseResponse>> {
  const query = new URLSearchParams({
    limit: String(input.limit ?? 25),
    offset: String(input.offset ?? 0),
  });
  if (input.account?.trim()) {
    query.set("account", input.account.trim());
    if (input.fuzzy) query.set("fuzzy", "true");
  }
  if (input.sort_by) query.set("sort_by", input.sort_by);
  return apiRequest<PageResponse<LeaseResponse>>(`${basePath}/${platformId}/leases?${query}`);
}

export async function deleteLease(platformId: string, account: string): Promise<void> {
  await apiRequest<void>(`${basePath}/${platformId}/leases/${encodeURIComponent(account)}`, {
    method: "DELETE",
  });
}
