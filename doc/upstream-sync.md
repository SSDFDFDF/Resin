# 上游同步说明

## 目的

当前仓库从 `upstream` 分叉的基线提交为：

- `0b2040d287157f933833bdb33ea96c25b02e71f5`

从 `2026-04-19` 这次同步开始，不再把上游更新当作“整段硬合并”。后续统一采用“维护审查游标 + 选择性吸收必要修复 + 保留本地功能决策”的方式同步。

当前同步状态：

- 分叉基线：`0b2040d287157f933833bdb33ea96c25b02e71f5`
- 已审查上游游标：`24e652666d6c02b8b1ff501fbf59a3ca2783f6b4`
- 上游 remote：`upstream`
- 上游分支：`master`

## 同步原则

1. 优先同步稳定性修复、兼容性修复、运维体验修复。
2. 上游功能更新如果与本地实现存在分歧，以本地行为为准，只融合其中必要的修复意图。
3. 高耦合的大功能簇可以整组跳过，避免为当前分支引入过高合并成本。
4. 每次同步完成后，必须更新本文件中的：
   - 已审查上游游标
   - 已融合提交或手工适配项
   - 已跳过提交及原因

## 固定流程

1. 拉取上游并查看新差异：

   ```bash
   make upstream-sync-audit
   ```

   如果分叉基线或审查游标发生变化，可显式覆盖：

   ```bash
   make upstream-sync-audit \
     SYNC_BASE=<fork-base> \
     REVIEWED_UPSTREAM_HEAD=<last-reviewed-upstream-head>
   ```

2. 先只看“上次审查之后新增的上游提交”：

   ```bash
   git log --oneline REVIEWED_UPSTREAM_HEAD..upstream/master
   ```

3. 将每个上游提交分成三类：
   - `merge`：可直接并入的必要修复
   - `adapt`：修复意图需要，但要按本地实现手工融合
   - `skip`：非必要更新、与本地路线冲突、或依赖过重

4. 应用顺序建议：
   - 能直接适配本地实现的，再考虑 cherry-pick
   - 只要本地已经形成不同 API、不同数据模型，优先手工融合行为，不强行跟随上游结构

5. 同步后必须校验：

   ```bash
   go test ./...
   cd webui && npm run build
   ```

6. 同步完成后，更新本文件与 `Makefile` 中的 `REVIEWED_UPSTREAM_HEAD`。

## 2026-04-19 同步记录

本次审查上游区间：

- `0b2040d287157f933833bdb33ea96c25b02e71f5..24e652666d6c02b8b1ff501fbf59a3ca2783f6b4`

本次已融合或已手工适配的上游修复：

- `3a067e9` `retry selected http status codes via proxy`
  - 已融合到本地 `RetryDownloader`，保留本地的订阅下载 UA 覆盖能力。
- `9da4151` `Parse Clash VLESS reality-opts and normalize WS ALPN`
  - 已手工融合到本地订阅解析器。
- `5a37b14` `Normalize Hysteria port ranges from subscriptions`
  - 已手工融合到本地订阅解析器。
- `998d7c8` `Ignore placeholder VLESS flow values`
  - 已手工融合到本地订阅解析器。
- `8126090` `fix(platform): unify unknown region filter behavior`
  - 已按本地 `region_filters + region_filter_invert` 模型手工融合，运行时和预览逻辑保持一致。
- `046e19c` `fix(webui): isolate node probe button pending states`
  - 已融合到本地 WebUI。
- `853a8ab` `fix(webui): isolate subscription refresh pending states`
  - 已融合到本地 WebUI。
- `1191a5c` `fix(webui): surface negated region filters in create flow`
  - 已部分吸收为本地文案提示；本地继续保留独立的 `region_filter_invert` 开关，不改成上游的 `!hk` 内联语法。

本次明确跳过的上游提交：

- `35279b8`、`0253f30`
  - 上游把地区反选做成了 `!hk` 这类内联语法；本地已存在显式 `region_filter_invert`，因此不跟随其数据模型变更。
- `79154cf`
  - 与本地“订阅下载支持自定义 user-agent”能力冲突，跳过。
- `876241a`
  - 上游把订阅下载默认 UA 固定为 `clash.meta`；本地保留可配置默认值和订阅级覆盖，跳过。
- `73ce2b9`、`62982dc`、`adb782a`、`24e6526`
  - 非必要 UI/文档更新，本次跳过。
- `741e6b3`
  - 属于功能性增强，单独评估，不在本次必要修复范围内。
- `0532205`、`2eccda3`、`8f9c2d4`、`e524c0e`、`96945e5`、`c2b2189`、`9fec172`、`6464db5`、`f61ffab`、`9a3d1e2`
  - 属于 SOCKS5 入站与 tunnel 相关的大功能簇，依赖重、合并成本高，且当前分支暂无必须引入的需求，因此整组跳过。

当前继续以本地实现为准的能力：

- 租约管理页签及配套后端能力
- 手动粘性租约模式与更严格的租约校验
- 平台正则反向过滤
- 订阅与下载器 user-agent 覆盖能力
- anytls 支持与本地平台筛选交互流程
