import { Info } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Input } from "../../components/ui/Input";
import { Switch } from "../../components/ui/Switch";
import { useI18n } from "../../i18n";
import { listSubscriptions } from "../subscriptions/api";

type PlatformSubscriptionFilterFieldProps = {
  idPrefix: string;
  selectedIds: string[];
  invert: boolean;
  onSelectedIdsChange: (ids: string[]) => void;
  onInvertChange: (invert: boolean) => void;
};

const EMPTY_IDS: string[] = [];

export function PlatformSubscriptionFilterField({
  idPrefix,
  selectedIds,
  invert,
  onSelectedIdsChange,
  onInvertChange,
}: PlatformSubscriptionFilterFieldProps) {
  const { t } = useI18n();
  const [keyword, setKeyword] = useState("");
  const trimmedKeyword = keyword.trim();
  const selected = Array.isArray(selectedIds) ? selectedIds : EMPTY_IDS;
  const selectedSet = new Set(selected);

  const subscriptionsQuery = useQuery({
    queryKey: ["subscriptions", "platform-filter-picker", trimmedKeyword],
    queryFn: () => listSubscriptions({ limit: 1000, offset: 0, keyword: trimmedKeyword || undefined }),
    staleTime: 30_000,
  });

  const subscriptions = subscriptionsQuery.data?.items ?? [];
  const visibleIds = new Set(subscriptions.map((item) => item.id));
  const missingIds = selected.filter((id) => !visibleIds.has(id));

  const toggleSubscription = (id: string) => {
    if (selectedSet.has(id)) {
      onSelectedIdsChange(selected.filter((item) => item !== id));
      return;
    }
    onSelectedIdsChange([...selected, id]);
  };

  return (
    <div className="field-group field-span-2 platform-subscription-filter">
      <div className="platform-subscription-filter-head">
        <label className="field-label field-label-with-info">
          <span>{t("订阅源过滤")}</span>
          <span
            className="subscription-info-icon"
            title={t("按订阅源隔离当前平台可用节点")}
            aria-label={t("按订阅源隔离当前平台可用节点")}
            tabIndex={0}
          >
            <Info size={13} />
          </span>
        </label>
        <label className="subscription-inline-filter" htmlFor={`${idPrefix}-subscription-filter-invert`} style={{ flexDirection: "row", alignItems: "center" }}>
          <Switch
            id={`${idPrefix}-subscription-filter-invert`}
            checked={invert}
            onChange={(event) => onInvertChange(event.target.checked)}
          />
          <span>{invert ? t("排除选中订阅") : t("仅包含选中订阅")}</span>
        </label>
      </div>

      <Input
        value={keyword}
        onChange={(event) => setKeyword(event.target.value)}
        placeholder={t("搜索订阅源")}
        aria-label={t("搜索订阅源")}
      />

      <div className="platform-subscription-filter-list" aria-label={t("订阅源过滤列表")}>
        {subscriptionsQuery.isLoading ? <p className="muted">{t("正在加载订阅数据...")}</p> : null}
        {!subscriptionsQuery.isLoading && subscriptions.length === 0 ? (
          <p className="muted">{trimmedKeyword ? t("没有匹配的订阅") : t("暂无订阅源")}</p>
        ) : null}

        {subscriptions.map((subscription) => {
          const checked = selectedSet.has(subscription.id);
          return (
            <label key={subscription.id} className="platform-subscription-filter-option">
              <input type="checkbox" checked={checked} onChange={() => toggleSubscription(subscription.id)} />
              <span>
                <strong>{subscription.name}</strong>
                <small>{subscription.enabled ? t("启用") : t("已停用")} · {subscription.node_count}</small>
              </span>
            </label>
          );
        })}

        {missingIds.map((id) => (
          <label key={id} className="platform-subscription-filter-option platform-subscription-filter-option-missing">
            <input type="checkbox" checked onChange={() => toggleSubscription(id)} />
            <span>
              <strong>{t("未知订阅")}</strong>
              <small>{id}</small>
            </span>
          </label>
        ))}
      </div>
    </div>
  );
}
