import { type ColumnDef } from "@tanstack/react-table";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Search, Trash2 } from "lucide-react";
import { useState } from "react";
import { Badge } from "../../components/ui/Badge";
import { Button } from "../../components/ui/Button";
import { DataTable } from "../../components/ui/DataTable";
import { Input } from "../../components/ui/Input";
import { OffsetPagination } from "../../components/ui/OffsetPagination";
import { Select } from "../../components/ui/Select";
import { ToastContainer } from "../../components/ui/Toast";
import { useToast } from "../../hooks/useToast";
import { useI18n } from "../../i18n";
import { formatApiErrorMessage } from "../../lib/error-message";
import { formatDateTime, formatRelativeTime } from "../../lib/time";
import { clearAllPlatformLeases, deleteLease, listLeases } from "./api";
import type { LeaseResponse } from "./types";

type LeaseTabProps = {
  platformId: string;
  platformName: string;
};

const PAGE_SIZE_OPTIONS = [25, 50, 100] as const;
const SORT_OPTIONS = [
  { value: "account", label: "账号" },
  { value: "expiry", label: "过期时间" },
  { value: "last_accessed", label: "最后访问" },
] as const;

export function LeaseTab({ platformId, platformName }: LeaseTabProps) {
  const { t } = useI18n();
  const { toasts, showToast, dismissToast } = useToast();
  const queryClient = useQueryClient();

  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<"account" | "expiry" | "last_accessed">("account");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState<number>(25);

  const leasesQuery = useQuery({
    queryKey: ["platform-leases", platformId, page, pageSize, search, sortBy],
    queryFn: () =>
      listLeases(platformId, {
        limit: pageSize,
        offset: page * pageSize,
        account: search || undefined,
        fuzzy: search ? true : undefined,
        sort_by: sortBy,
      }),
    refetchInterval: 15_000,
    placeholderData: (prev) => prev,
  });

  const leases = leasesQuery.data?.items ?? [];
  const total = leasesQuery.data?.total ?? 0;
  const totalPages = Math.ceil(total / pageSize);

  const deleteMutation = useMutation({
    mutationFn: (account: string) => deleteLease(platformId, account),
    onSuccess: (_, account) => {
      const nextTotal = Math.max(0, total - 1);
      const nextLastPage = Math.max(0, Math.ceil(nextTotal / pageSize) - 1);
      if (page > nextLastPage) {
        setPage(nextLastPage);
      }
      queryClient.invalidateQueries({ queryKey: ["platform-leases"] });
      queryClient.invalidateQueries({ queryKey: ["platform-monitor"] });
      showToast("success", t("租约 {{account}} 已删除", { account }));
    },
    onError: (error) => {
      showToast("error", formatApiErrorMessage(error, t));
    },
  });

  const clearAllMutation = useMutation({
    mutationFn: () => clearAllPlatformLeases(platformId),
    onSuccess: () => {
      setPage(0);
      queryClient.invalidateQueries({ queryKey: ["platform-leases"] });
      queryClient.invalidateQueries({ queryKey: ["platform-monitor"] });
      showToast("success", t("平台 {{name}} 的所有租约已清除", { name: platformName }));
    },
    onError: (error) => {
      showToast("error", formatApiErrorMessage(error, t));
    },
  });

  const handleDeleteLease = (account: string) => {
    if (window.confirm(t("确认删除租约 {{account}}？", { account }))) {
      deleteMutation.mutate(account);
    }
  };

  const handleClearAll = () => {
    if (window.confirm(t("确认清除平台 {{name}} 的所有租约？", { name: platformName }))) {
      clearAllMutation.mutate();
    }
  };

  const handleSearchChange = (value: string) => {
    setSearch(value);
    setPage(0);
  };

  const handleSortChange = (value: string) => {
    setSortBy(value as "account" | "expiry" | "last_accessed");
    setPage(0);
  };

  const columns: ColumnDef<LeaseResponse, unknown>[] = [
    {
      accessorKey: "account",
      header: () => t("账号"),
      cell: (info) => <span>{info.getValue<string>()}</span>,
    },
    {
      accessorKey: "node_tag",
      header: () => t("节点名"),
      cell: (info) => info.getValue<string>(),
    },
    {
      accessorKey: "egress_ip",
      header: () => t("出口 IP"),
      cell: (info) => <span>{info.getValue<string>()}</span>,
    },
    {
      accessorKey: "expiry",
      header: () => t("过期时间"),
      cell: (info) => formatDateTime(info.getValue<string>()),
    },
    {
      accessorKey: "last_accessed",
      header: () => t("最后访问"),
      cell: (info) => formatRelativeTime(info.getValue<string>()),
    },
    {
      id: "status",
      header: () => t("状态"),
      cell: (info) => {
        const row = info.row.original;
        return row.unavailable_since ? (
          <Badge variant="warning">{t("不可用")}</Badge>
        ) : (
          <Badge variant="success">{t("活跃")}</Badge>
        );
      },
    },
    {
      id: "actions",
      header: () => t("操作"),
      cell: (info) => {
        const account = info.row.original.account;
        return (
          <Button
            variant="danger"
            size="sm"
            onClick={(e) => {
              e.stopPropagation();
              handleDeleteLease(account);
            }}
            disabled={deleteMutation.isPending}
          >
            <Trash2 size={14} />
            {t("删除")}
          </Button>
        );
      },
    },
  ];

  return (
    <div className="lease-tab">
      <ToastContainer toasts={toasts} onDismiss={dismissToast} />

      <div className="lease-tab-toolbar">
        <label className="search-box lease-tab-search" htmlFor="lease-account-search">
          <Search size={16} />
          <Input
            id="lease-account-search"
            placeholder={t("账号搜索")}
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
          />
        </label>

        <div className="lease-tab-actions">
          <Select value={sortBy} onChange={(e) => handleSortChange(e.target.value)} aria-label={t("排序")}>
            {SORT_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {t(opt.label)}
              </option>
            ))}
          </Select>

          <Button variant="danger" size="sm" onClick={() => void handleClearAll()} disabled={clearAllMutation.isPending}>
            {clearAllMutation.isPending ? t("清除中...") : t("清除所有租约")}
          </Button>
        </div>
      </div>

      {leasesQuery.isLoading && !leases.length ? (
        <p className="muted">{t("正在加载...")}</p>
      ) : total === 0 ? (
        <p className="muted">{t("暂无租约")}</p>
      ) : (
        <DataTable<LeaseResponse>
          data={leases}
          columns={columns}
          className="data-table-leases"
          getRowId={(row) => row.account}
        />
      )}

      {total > 0 ? (
        <OffsetPagination
          page={page}
          totalPages={totalPages}
          totalItems={total}
          pageSize={pageSize}
          pageSizeOptions={PAGE_SIZE_OPTIONS}
          onPageChange={setPage}
          onPageSizeChange={(size) => {
            setPageSize(size);
            setPage(0);
          }}
        />
      ) : null}
    </div>
  );
}
