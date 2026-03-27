import type {
  PlatformAllocationPolicy,
  PlatformEmptyAccountBehavior,
  PlatformManualUnavailableAction,
  PlatformMissAction,
  PlatformStickyLeaseMode,
} from "./types";

export const allocationPolicies: PlatformAllocationPolicy[] = [
  "BALANCED",
  "PREFER_LOW_LATENCY",
  "PREFER_IDLE_IP",
];

export const missActions: PlatformMissAction[] = ["TREAT_AS_EMPTY", "REJECT"];

export const emptyAccountBehaviors: PlatformEmptyAccountBehavior[] = [
  "RANDOM",
  "FIXED_HEADER",
  "ACCOUNT_HEADER_RULE",
];

export const stickyLeaseModes: PlatformStickyLeaseMode[] = ["TTL", "MANUAL"];

export const manualUnavailableActions: PlatformManualUnavailableAction[] = ["HOLD", "AUTO_CLEAN"];

export const allocationPolicyLabel: Record<PlatformAllocationPolicy, string> = {
  BALANCED: "均衡",
  PREFER_LOW_LATENCY: "优先低延迟",
  PREFER_IDLE_IP: "优先空闲出口 IP",
};

export const missActionLabel: Record<PlatformMissAction, string> = {
  TREAT_AS_EMPTY: "按空账号处理",
  REJECT: "拒绝代理请求",
};

export const emptyAccountBehaviorLabel: Record<PlatformEmptyAccountBehavior, string> = {
  RANDOM: "随机路由",
  FIXED_HEADER: "提取指定请求头作为 Account",
  ACCOUNT_HEADER_RULE: "按照全局请求头规则提取 Account",
};

export const stickyLeaseModeLabel: Record<PlatformStickyLeaseMode, string> = {
  TTL: "TTL 粘性租约",
  MANUAL: "手动长租约",
};

export const manualUnavailableActionLabel: Record<PlatformManualUnavailableAction, string> = {
  HOLD: "保持租约并等待原 IP 恢复",
  AUTO_CLEAN: "自动清理租约并重新分配",
};
