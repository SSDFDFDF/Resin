export type PlatformMissAction = "TREAT_AS_EMPTY" | "REJECT";
export type PlatformEmptyAccountBehavior = "RANDOM" | "FIXED_HEADER" | "ACCOUNT_HEADER_RULE";
export type PlatformAllocationPolicy = "BALANCED" | "PREFER_LOW_LATENCY" | "PREFER_IDLE_IP";
export type PlatformStickyLeaseMode = "TTL" | "MANUAL";
export type PlatformManualUnavailableAction = "HOLD" | "AUTO_CLEAN";

export type Platform = {
  id: string;
  name: string;
  sticky_ttl: string;
  sticky_lease_mode: PlatformStickyLeaseMode;
  manual_unavailable_action: PlatformManualUnavailableAction;
  manual_unavailable_grace: string;
  regex_filters: string[];
  regex_filter_invert: boolean;
  region_filters: string[];
  region_filter_invert: boolean;
  routable_node_count: number;
  reverse_proxy_miss_action: PlatformMissAction;
  reverse_proxy_empty_account_behavior: PlatformEmptyAccountBehavior;
  reverse_proxy_fixed_account_header: string;
  allocation_policy: PlatformAllocationPolicy;
  updated_at: string;
};

export type PageResponse<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type LeaseResponse = {
  platform_id: string;
  account: string;
  node_hash: string;
  node_tag: string;
  egress_ip: string;
  expiry: string;
  last_accessed: string;
  unavailable_since?: string;
};

export type PlatformCreateInput = {
  name: string;
  sticky_ttl?: string;
  sticky_lease_mode?: PlatformStickyLeaseMode;
  manual_unavailable_action?: PlatformManualUnavailableAction;
  manual_unavailable_grace?: string;
  regex_filters?: string[];
  regex_filter_invert?: boolean;
  region_filters?: string[];
  region_filter_invert?: boolean;
  reverse_proxy_miss_action?: PlatformMissAction;
  reverse_proxy_empty_account_behavior?: PlatformEmptyAccountBehavior;
  reverse_proxy_fixed_account_header?: string;
  allocation_policy?: PlatformAllocationPolicy;
};

export type PlatformUpdateInput = {
  name?: string;
  sticky_ttl?: string;
  sticky_lease_mode?: PlatformStickyLeaseMode;
  manual_unavailable_action?: PlatformManualUnavailableAction;
  manual_unavailable_grace?: string;
  regex_filters?: string[];
  regex_filter_invert?: boolean;
  region_filters?: string[];
  region_filter_invert?: boolean;
  reverse_proxy_miss_action?: PlatformMissAction;
  reverse_proxy_empty_account_behavior?: PlatformEmptyAccountBehavior;
  reverse_proxy_fixed_account_header?: string;
  allocation_policy?: PlatformAllocationPolicy;
};
