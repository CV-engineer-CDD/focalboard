# Boards 任务 ID 与 loong64 插件开发记录

本文记录从修改 Focalboard/Boards 源码开始，到生成 `linux-loong64`
Mattermost Boards 插件包期间的需求演进、实现调整、问题修复和构建验证。

## 背景

使用 Mattermost Boards 管理项目时，原版卡片只有内部 block/card ID。这类 ID
是长字符串，不适合写入代码提交、构建产物、实验日志或性能分析记录。时间久了，
代码变更、Boards 任务和当时的构建环境很难对应起来。

本次改动的目标是：

- 给每张卡片增加稳定、可读的引用 ID。
- 旧卡片也要能迁移到可读编号，不能继续暴露内部字符串。
- 只构建 `linux-loong64` 插件。
- 不破坏 Mattermost 7.10/7.11 Boards 原有使用流程。

## 用户需求与决策摘要

本次开发不是一次性需求，而是在实际部署、导入插件、打开 board、新建/删除/恢复卡片
过程中逐步暴露问题并迭代。下面按对话中的主要诉求记录：

- 最初诉求：为 Boards 每个任务增加唯一、短小、可读的 ID，方便在代码、提交记录、
  构建产物和项目记录中引用，避免时间久了代码和 board 卡片对不上。
- 构建诉求：最终只需要能用于 Mattermost 插件上传的 `linux-loong64` 二进制插件包，
  并保留能回退的旧版本压缩包。
- 兼容诉求：旧卡片不能继续显示内部字符串；老数据也要进入数字编号序列，并且迁移
  过程不能冲突。
- 全局追踪诉求：单个 board 内编号不够，跨 board 也需要一个整体唯一 ID，方便在
  代码层面引用和跟踪。
- 性能诉求：原版没有明显卡顿，引入编号后新建、删除、打开整板不能出现明显额外延迟。
- 插件升级诉求：上传压缩包导入插件时不能因为 manifest 版本不变导致 Mattermost
  报“无法升级时重启插件”。
- 删除恢复诉求：既然后端支持 soft delete 和恢复，UI 中也要能查看已删除卡片并恢复，
  不应要求用户手写 API 或操作数据库。
- 永久删除诉求：`Deleted cards` 中还需要二次确认后的永久删除，永久删除后不再出现在
  已删除列表，也不能再恢复。
- 显示诉求：外部卡片卡面、表格、画廊、日历、详情标题应显示全局 ID；详情属性区显示
  board 内 ID，避免跨 board 引用时看错编号。
- 稳定性诉求：`Deleted cards` 要显示真实标题、真实 ID、真实删除时间；恢复、新建、
  删除不能出现点一次无响应、再点一次才成功的假象。
- 编号释放疑问：你提出“永久删除后的 ID 是否释放给新卡使用”。我的建议和最终决策是
  不释放，原因是本项目的核心价值是长期引用稳定。释放编号会让旧代码注释、commit、
  构建日志中的编号将来可能指向另一张新卡，破坏唯一引用。最终规则是 soft delete
  和 permanent delete 都不释放编号，新卡始终从历史最大编号继续递增。

因此，最终版本接受“编号中间会有空洞”的代价，换取跨时间维度的唯一性和可追溯性。

## 版本变更记录

### `7.11.0-taskid-boardonly-linux-loong64`

- 用途：保留的可回退版本。
- 功能：只提供 board 内短编号 `#N`。
- 修复：新建卡片、复制卡片、旧卡片迁移都会生成 board 内数字编号。
- 限制：没有跨 board 的全局 ID；不同 board 中可能同时存在 `#1`、`#2`。
- 产物：
  - `focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz`
  - `focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz.zst`

### `7.11.1-taskid-linux-loong64`

- 功能：在 board 内 `#N` 基础上增加跨 board 的 `globalTaskId`，显示为 `G-N`。
- 功能：`globalTaskId` 同时写入 `fields.globalTaskId` 和 `fields.properties.__globalTaskId`。
- 功能：卡片详情属性区显示只读 `Global ID`。
- 修复：插件 manifest 版本从 `7.11.0` 升到 `7.11.1`，避免同版本上传时 Mattermost 无法正常升级重启插件。
- 问题：最初为了避免全库扫描，缺失全局计数器时从毫秒时间戳起步，可能生成 `G-1780519393936` 这种不可读编号。
- 问题：全局迁移和 board 打开路径仍有性能压力。

### `7.11.2-taskid-linux-loong64`

- 修复：停止使用毫秒时间戳作为全局 ID 起点。
- 修复：如果系统设置中已存在时间戳形态计数器，会扫描已有正常 `G-N` 并重置为正常最大值。
- 修复：历史上已生成的 `G-1780...` 这类时间戳 ID 会被视为无效旧值，在对应 board 迁移时重新分配正常 `G-N`。
- 保持：新卡、复制卡仍由服务端生成 `#N` 和 `G-N`，客户端传入值会被覆盖。

### `7.11.3-taskid-linux-loong64`

- 修复：软删除卡片不再占用后续编号。
- 修复：如果删除的是当前最高 `#N` 或 `G-N`，内存缓存和全局系统计数器会回退，下一张新卡可以复用该最高号。
- 修复：恢复已删除卡片时，如果原编号已被新卡复用，恢复卡片会自动获得新的不冲突编号。
- 行为变化：编号唯一范围明确为“当前未删除卡片”；历史日志中如果只记录短 ID，删除复用后可能出现旧记录和新卡共用同一短 ID 的情况。

### `7.11.4-taskid-linux-loong64`

- 功能：board 顶部三点菜单增加 `Deleted cards` 入口。
- 功能：弹窗列出当前 board 已删除卡片，显示 board 内 `#N`、全局 `G-N`、标题和删除时间。
- 功能：弹窗中可点击 `Restore` 恢复卡片。
- 修复：前端 Redux store 新增 `deletedCards`，避免已删除卡片混入正常看板/表格，同时能在恢复弹窗中展示。
- 修复：恢复操作调用已有 `undeleteBlock` API，并把返回的卡片写回前端状态。
- 验证：`webapp npm run check`、主 webapp `npm run pack`、插件 webapp `npm run build` 均通过。
- 已知：当前 loong64 环境无法运行 Jest，原因是 `@swc/core` 没有 loong64 Linux native binding；已同步更新相关 snapshot 文本。

### `7.11.5-taskid-linux-loong64`

- 功能：`Deleted cards` 弹窗中新增 `Permanently delete` 操作。
- 功能：用户点击永久删除后必须二次确认，确认后该卡片从已删除列表移除，不能再通过 `Deleted cards` 恢复。
- 实现：新增 `DELETE /api/v2/boards/{boardID}/blocks/{blockID}/purge` 接口，只允许对已经 soft delete 的 block 执行永久删除。
- 实现：存储层递归收集该卡片在 `blocks_history` 中的子块，随后从 `blocks` 和 `blocks_history` 删除对应记录。
- 防护：服务端拒绝对未删除卡片执行永久删除，避免把普通删除流程误变成物理删除。
- 验证：`webapp npm run check` 通过；`go test ./app -run '^(TestDeleteBlock|TestUndeleteBlock|TestPermanentlyDeleteBlock)$' -count=1` 通过；`go test ./api -run TestNonExistent -count=1` 通过。
- 已知：`go test ./services/store/sqlstore` 在当前 loong64 环境仍受 `modernc.org/libc` build constraints 限制，无法作为本地验证项。

### `7.11.6-taskid-linux-loong64`

- 需求修正：用户反馈 `7.11.3` 的“删除最高号后复用编号”虽然能填补空洞，但会破坏代码、提交记录和 Boards 卡片之间的长期引用稳定性。
- 设计结论：软删除和永久删除都不再释放 `taskId` / `globalTaskId`；永久删除只删除卡片与恢复历史，不把编号放回可用池。
- 功能调整：卡片外侧显示跨 board 全局 ID，`G-30392` 展示为 `#30392`。
- 功能调整：卡片详情属性区把 `Global ID` 改为 `Board ID`，显示 board 内编号数字，例如 `471`。
- 修复：`Deleted cards` 弹窗不再被 websocket 删除占位块覆盖完整卡片信息，避免只显示内部字符串和 `Untitled`。
- 修复：`Deleted cards` 从后端读取最新 deleted card history，删除时间使用真实历史 `deleteAt`。
- 修复：恢复、删除、新建等 mutator 路径检查 HTTP 状态，避免失败响应被前端当作成功处理造成“点一次没反应”。
- 实现：新增 board 级持久计数器 `focalboard_card_task_id_max_{boardID}`，保证永久删除历史后重启也不会复用 board 内编号。
- 实现：全局计数器初始化时会取 active cards 与 deleted card history 的最大值，修复从 `7.11.3` 升级后 deleted 最高号被遗忘的问题。
- 验证：`webapp npm run check` 通过；`go test ./app -run '^(TestDeleteBlock|TestUndeleteBlock|TestPermanentlyDeleteBlock|TestCardTaskIDLifecycleUsesUniqueIDs|TestCreateCard|TestEnsureCardTaskIDs|TestNextCardGlobalTaskIDResetsTimestampCounter)$' -count=1` 通过；`go test ./api -run TestNonExistent -count=1` 通过。

### `7.11.7-taskid-linux-loong64`

- 问题：用户继续验证后反馈，部分情况下实际行为仍不是“软删除和永久删除都不释放编号”。
- 原因：`7.11.6` 虽然取消了 soft delete 主路径的计数器回退，但恢复卡片时仍按“活动卡片最大编号”重写计数器；永久删除前的保留逻辑在缓存未加载或被删卡编号较小时，也可能把持久计数器降低。
- 修复：恢复卡片时只用活动卡片判断编号冲突，但计数器必须取活动卡片、deleted card history、持久计数器和内存缓存中的最大值。
- 修复：永久删除前只允许抬高 board/global 计数器，绝不允许用被永久删除卡片的较小编号覆盖更大的历史最大值。
- 修复：恢复路径同步写入 board 级持久计数器 `focalboard_card_task_id_max_{boardID}`，避免恢复后重启再发号时丢失历史最大值。
- 清理：删除旧的“删除后刷新/回退计数器”函数，避免以后误用重新引入编号复用。
- 验证：`go test ./app -run '^(TestUndeleteBlock|TestPermanentlyDeleteBlock|TestDeleteBlock|TestCardTaskIDLifecycleUsesUniqueIDs|TestCreateCard|TestEnsureCardTaskIDs|TestNextCardGlobalTaskIDResetsTimestampCounter)$' -count=1` 通过；`go test ./model -run '^(TestBlock2Card|TestCard2BlockIncludesTaskID)$' -count=1` 通过；`go test ./api -run TestNonExistent -count=1` 通过。

## 需求演进

### 第一阶段：board 内唯一 ID

最初需求是给每张卡片增加唯一 ID，方便在代码中引用和跟踪。实现选择为
board 内编号：

- 字段：`fields.taskId`
- 格式：`#1`, `#2`, `#3`
- 唯一范围：同一个 board 内
- 用途：日常看板沟通、单个项目 board 内的短引用

旧卡片迁移规则：

- 先保留已有合法 `#N`。
- 重复的 `#N` 只保留按 `createAt` 和 block ID 排序最早的一张。
- 缺失、非法值、内部 card ID 字符串都视为需要迁移。
- 新编号从当前最大 N 之后继续分配，不填补空洞，避免破坏已有引用。

### 第二阶段：真实创建路径修复

第一次部署后发现新卡片仍显示长字符串。原因是 Boards 前端新建卡片实际走的是：

```text
POST /boards/{boardID}/blocks
```

也就是直接插入 `card` block，而不是最初重点修改的 `/cards` 创建接口。

修复点：

- 在 `InsertBlockAndNotify` 和 `InsertBlocksAndNotify` 中识别 `TypeCard`。
- 服务端强制覆盖客户端传入的 `taskId`。
- 新卡、复制卡都由服务端生成 `#N`。

### 第三阶段：旧数据全量读取路径修复

第二次部署后，新卡已经是数字 ID，但旧卡仍显示字符串。原因是打开 board 时前端
请求全量 blocks：

```text
GET /boards/{boardID}/blocks?all=...
```

对应 app 层 `GetBlocksForBoard`，此前没有触发旧卡迁移。

修复点：

- 在 `GetBlocksForBoard` 返回 blocks 前执行旧卡 `taskId` 补齐。
- 打开旧 board 时会自动把旧字符串迁移成 `#N`。

### 第四阶段：跨 board 全局 ID

后续需求是除了 board 内 `#N`，还需要一个在所有 board 中唯一的整体 ID，用于跨
项目、跨 board、代码提交和构建产物追踪。

这个需求是合理的，但不应该替代 board 内 ID。最终设计为双 ID：

- `taskId`: board 内唯一，格式 `#N`
- `globalTaskId`: 全局唯一，格式 `G-N`

`globalTaskId` 同时写入：

- `fields.globalTaskId`
- `fields.properties.__globalTaskId`

前端在卡片详情属性区以只读属性形式显示 `Global ID`。不自动修改用户的 board
property template，避免污染用户自定义列配置。

### 第五阶段：性能修复

引入旧数据迁移和全局 ID 后，删除/新建卡片出现约 0.5-1 秒延迟。原因是迁移逻辑
在打开 board、新建、复制等路径中反复扫描 blocks，尤其全局 ID 会扫描所有 board
的卡片。

性能修复：

- 每个 board 的旧 `taskId` 迁移只在当前进程内执行一次。
- 全局 `globalTaskId` 不再通过扫描所有 board 恢复最大值。
- 全局编号改用 `system_settings` 中的持久计数器 `focalboard_card_global_task_id_max`。
- 如果计数器不存在，首次发号时扫描一次已有卡片，按最高的正常 `G-N` 初始化。
- 上一版错误生成的毫秒时间戳形态 ID，例如 `G-1780519393936`，视为无效旧值，
  在对应 board 迁移时重新分配为正常序列号。
- 旧 `globalTaskId` 只在当前打开或写入的 board 内懒迁移，并在进程内按 board 缓存。
- 打开整板时复用已经取回的 board blocks 做迁移，避免打开前额外查询一次当前 board 卡片。
- 新建/复制卡片直接从持久计数器递增，不再重复全量扫描。
- 软删除卡片不再参与后续最大编号计算；如果删除的是当前最高号，缓存和全局计数器会回退。
- 恢复已删除卡片时，如果原编号已被新卡片复用，恢复的卡片会自动获得新的不冲突编号。
- board 内编号仍使用 board 级锁。
- 全局编号使用全局锁。
- 缓存 map 增加独立锁，避免不同 board 并发写缓存导致 Go map race。

### 第六阶段：已删除卡片 UI 恢复

在 `7.11.3` 中，为了满足“删除最高编号后下一张新卡接替该编号”的需求，服务端把
软删除卡片排除在编号占用范围之外，并补了恢复冲突时自动重分配 ID 的逻辑。
随后产生了新的使用问题：既然后端支持恢复，那么 UI 中也需要一个可见入口，否则用户
不知道如何恢复已删除卡片。

需求：

- 能在 board UI 中看到当前 board 已删除卡片。
- 能从 UI 直接恢复卡片，不需要手写 API 或操作数据库。
- 恢复后仍遵守 `7.11.3` 的编号规则：如果原 `#N` / `G-N` 已被新卡复用，恢复卡片
  自动获得新的不冲突编号。
- 已删除卡片不能重新混入正常看板、表格、画廊、日历视图。

实现：

- 复用后端已有接口 `POST /boards/{boardID}/blocks/{blockID}/undelete`。
- 复用前端已有 `octoClient.undeleteBlock`，在 `mutator` 中新增显式
  `undeleteBlock` 方法，把恢复后的 block 写回 Redux。
- Redux `cards` store 新增 `deletedCards`，专门保存软删除卡片。
- `updateCards` / `loadBoardData` / `initialReadOnlyLoad` 遇到 `deleteAt != 0`
  的 card 时写入 `deletedCards`，并从正常 `cards` / `templates` 中移除。
- 正常 selector 仍只返回未删除卡片，避免影响 board 原功能。
- board 顶部三点菜单增加 `Deleted cards ({count})`。
- 新增 `DeletedCardsDialog`，显示 `#N`、`G-N`、标题、删除时间和 `Restore` 按钮。
- 恢复失败时显示错误 flash message，成功时显示正常提示。

验证与构建过程：

- `webapp npm run check` 通过。
- 主 webapp `npm run pack` 通过，只出现原有 bundle size warning。
- 插件 webapp `npm run build` 通过，只出现原有 bundle size warning。
- 插件 webapp `npm run lint` 通过。
- `mattermost-plugin/webapp npm run check-types` 在当前仓库/loong64 环境失败，
  错误来自既有 Mattermost webapp 类型定义和 React 类型重复，不指向本次新增文件。
- Jest 在当前 loong64 环境失败，原因是 `@swc/core` 没有 Linux loong64 native
  binding；因此无法在本机直接跑 `viewHeaderActionsMenu.test.tsx`，已手动同步
  snapshot 中新增的 `Deleted cards (1)` 菜单项。

构建问题与处理：

- 运行 `mattermost-plugin/webapp npm run check-types` 后，`tsc` 会在
  `mattermost-plugin/webapp/dist` 中生成大量 `.js` / `.map` 和测试编译产物。
- 如果不清理直接 `make bundle`，这些临时产物会被打进插件包。
- 处理方式是清理 `webapp/pack`、`webapp/dist`、`mattermost-plugin/webapp/dist`、
  `mattermost-plugin/dist`，然后按正确顺序重新执行：
  `webapp npm run pack`、`mattermost-plugin/webapp npm run build`、`make bundle`。
- 最终校验确认 `focalboard-7.11.4.tar.gz` 中没有 `webapp/dist/webapp`、
  `webapp/dist/mattermost-plugin` 或 `*.test.js` 编译产物。

### 第七阶段：已删除卡片永久删除

恢复功能可用后，继续出现一个实际管理需求：部分已删除卡片不希望长期留在
`Deleted cards` 弹窗中，也不希望再能被恢复。因此增加永久删除能力。

设计边界：

- 普通删除仍然是 soft delete，保留恢复能力。
- 永久删除只出现在 `Deleted cards` 弹窗中。
- 永久删除前必须二次确认。
- 服务端只允许永久删除最新历史状态已经是 deleted 的 block。
- 永久删除后，该卡片及其历史子块从 `blocks_history` 删除，因此不会再进入恢复列表。

本次修改：

- `server/api/blocks.go`
  - 新增 `DELETE /boards/{boardID}/blocks/{blockID}/purge`。
  - 保持与普通删除相同的 `PermissionManageBoardCards` 权限。
  - 校验 block 历史记录属于当前 board。
- `server/app/blocks.go`
  - 新增 `PermanentlyDeleteBlock`。
  - 拒绝未删除 block，返回 bad request。
  - 删除成功后广播 block delete，让已打开客户端同步移除。
- `server/services/store/sqlstore/blocks.go`
  - 新增递归收集历史子块 ID 的逻辑。
  - 同时清理 `blocks` 和 `blocks_history`，覆盖极端情况下仍残留在 active 表中的记录。
- `webapp/src/components/viewHeader/deletedCardsDialog.tsx`
  - 每张已删除卡片增加 `Permanently delete` 按钮。
  - 点击后弹出确认框，确认后调用 mutator 永久删除。
  - 成功后从本地 `deletedCards` 状态移除。
- `webapp/src/store/cards.ts`
  - 新增 `removeDeletedCard` reducer。
- `webapp/src/octoClient.ts`、`webapp/src/mutator.ts`
  - 新增永久删除 API 调用封装。

验证情况：

```bash
cd webapp
npm run check

cd ../server
go test ./app -run '^(TestDeleteBlock|TestUndeleteBlock|TestPermanentlyDeleteBlock)$' -count=1
go test ./api -run TestNonExistent -count=1
```

结果：

- 前端 lint/stylelint 通过。
- App 层删除、恢复、永久删除测试通过。
- API 包编译通过。
- `sqlstore` 包在 loong64 上仍因 `modernc.org/libc` 对 loong64 缺少对应 build
  constraints 文件而无法本地运行，这属于此前已存在的 loong64 测试环境限制。

### 第八阶段：编号不复用、显示语义对换与 Deleted cards 修复

`7.11.5` 后继续测试时，出现了几类使用问题：

- `Deleted cards` 中有时恢复卡片第一次失败，第二次才成功。
- `Deleted cards` 中部分卡片只显示内部字符串和 `Untitled`，看不到真实标题和可读 ID。
- `Deleted cards` 中删除时间异常，多个卡片显示成同一个时间。
- 新建卡片、删除卡片偶发点击后无响应，需要再点一次。
- 外部卡片卡面显示的是 board 内编号，但用户更希望外部显示全局编号，详情属性显示 board 内编号。
- 对永久删除后是否释放编号存在疑问：如果释放，能填补空洞；如果不释放，能保证代码引用长期唯一。

最终取舍：

- 以“代码和 Boards 记录长期稳定关联”为最高优先级。
- `globalTaskId` 不能复用，否则旧 commit、构建日志或代码注释中的编号可能指向新任务。
- `taskId` 也不复用，保持规则一致，避免恢复、永久删除、重启后出现编号漂移。
- 因此删除 `#28447` 后，即使最大编号是 `#36666`，新卡仍从 `#36667` 继续；中间空洞是有意保留。
- 永久删除的含义是“卡片和恢复历史不可再恢复”，不是“编号进入可复用池”。

本次修改：

- 普通 soft delete 不再回退 board/global 计数器。
- `GetBlocksForBoard` 额外返回当前 board 最新 deleted card history，使 Deleted cards 弹窗拿到真实标题、字段和删除时间。
- 前端 `deletedCards` reducer 合并已有完整卡片数据与 websocket 删除事件，只用删除事件更新 `deleteAt`，避免占位块覆盖标题和字段。
- 新增 `displayCardGlobalID` / `displayCardBoardID` 前端 helper。
- 看板、表格、画廊、日历、卡片详情标题统一外显全局 ID。
- 卡片详情属性区从 `Global ID` 改为 `Board ID`。
- mutator 对新建、删除、恢复、永久删除的 HTTP response 做状态检查。
- board 内最大编号持久化到 `system_settings`，key 为 `focalboard_card_task_id_max_{boardID}`。
- 永久删除前会把被删卡片编号写入 board/global 最大计数器，确保历史删除后也不会复用。

验证情况：

```bash
cd webapp
npm run check

cd ../server
go test ./app -run '^(TestDeleteBlock|TestUndeleteBlock|TestPermanentlyDeleteBlock|TestCardTaskIDLifecycleUsesUniqueIDs|TestCreateCard|TestEnsureCardTaskIDs|TestNextCardGlobalTaskIDResetsTimestampCounter)$' -count=1
go test ./api -run TestNonExistent -count=1
```

结果：

- 前端 lint/stylelint 通过。
- App 层 ID 生命周期、删除、恢复、永久删除测试通过。
- API 包编译通过。

## 主要源码修改

服务端模型：

- `server/model/card.go`
  - 增加 `TaskID`
  - 增加 `GlobalTaskID`
  - `Card2Block` / `Block2Card` 双向转换字段

服务端 app 层：

- `server/app/cards.go`
  - board 内 `taskId` 生成、解析、迁移
  - 全局 `globalTaskId` 生成、解析、迁移
  - 重复编号去重
  - 迁移缓存和最大编号缓存
  - `properties.__globalTaskId` 同步

- `server/app/blocks.go`
  - `GetBlocks`
  - `GetBlocksForBoard`
  - `InsertBlockAndNotify`
  - `InsertBlocksAndNotify`
  - `DuplicateBlock`
  - 这些真实路径都会处理 card ID

- `server/app/app.go`
  - board 级 task ID 锁
  - 全局 ID 锁
  - 迁移状态和最大编号缓存

前端：

- `webapp/src/blocks/card.ts`
  - 增加 `globalTaskId` 类型

- `webapp/src/components/cardDetail/cardDetailProperties.tsx`
  - 卡片详情属性区显示只读 `Board ID`

- `webapp/src/store/cards.ts`
  - limited card 状态保留 `globalTaskId`
  - 增加 `deletedCards`，保存已删除卡片供恢复弹窗使用
  - 增加 `removeDeletedCard`，永久删除成功后从恢复列表移除
  - 删除事件与已有完整卡片合并，避免 websocket 删除占位块覆盖标题和 ID 字段

- `webapp/src/cardIDs.ts`
  - 统一外显全局 ID 和详情 board ID 的格式化

- `webapp/src/styles/main.scss`
  - ID 只读显示样式

- `webapp/src/mutator.ts`
  - 增加 `undeleteBlock`，调用恢复 API 并更新前端状态
  - 增加 `permanentlyDeleteBlock`，调用永久删除 API 并更新前端状态
  - 对新建、删除、恢复、永久删除接口做 HTTP 状态检查

- `webapp/src/octoClient.ts`
  - 增加 `/purge` 永久删除接口调用

- `webapp/src/components/viewHeader/viewHeaderActionsMenu.tsx`
  - board 顶部三点菜单增加 `Deleted cards ({count})`

- `webapp/src/components/viewHeader/deletedCardsDialog.tsx`
  - 已删除卡片列表与恢复按钮
  - 增加永久删除按钮和二次确认

- `webapp/src/components/viewHeader/deletedCardsDialog.scss`
  - 已删除卡片弹窗样式

插件/loong64：

- `mattermost-plugin/plugin.json`
  - 只保留 `linux-loong64`

- `mattermost-plugin/Makefile`
  - 增加 loong64 server build

- `server/services/store/sqlstore/sqlite_migration*.go`
  - loong64 下绕开不支持的 sqlite migration driver import

- `mattermost-plugin/go.mod`
  - 升级 `modernc.org/sqlite` / `modernc.org/libc`，解决 loong64 编译问题

## 遇到的问题

### 前端创建路径不是 `/cards`

现象：部署后新卡片显示内部字符串。

原因：Boards UI 直接插入 card block。

修复：在 block insert 路径生成 ID。

### 旧数据全量加载没有迁移

现象：新卡是数字 ID，旧卡仍是字符串。

原因：打开 board 时走 `GetBlocksForBoard`。

修复：`GetBlocksForBoard` 前触发迁移。

### `CreateCard` 与 block insert 双重加锁

问题：最初 `CreateCard` 自己分配 `taskId`，后续 block insert 也分配，存在逻辑分叉
和潜在死锁。

修复：统一由 block insert 路径分配 card ID。

### 全局 ID 引入性能问题

问题：全局扫描所有卡片成本高。

修复：全局 ID 使用 `system_settings` 持久计数器；打开 board 只迁移当前 board
中的旧卡片，不再扫描所有 board。

### loong64 sqlite 依赖编译问题

问题：旧 `modernc.org/libc` 在 loong64 下缺少若干 build target。

修复：

- 插件 go.mod 升级 modernc 依赖。
- Focalboard sqlite migration driver 在 `linux/loong64` 下使用 build tag 隔离。

### npm install/cwebp-bin 问题

问题：loongarch64 下 `cwebp-bin` 的 config.guess 不识别架构。

处理：

- 使用 `npm install --ignore-scripts` 安装依赖。
- 再分别执行 webapp 和插件 webapp 的 webpack build。

## 当前行为

新建卡片：

- 服务端覆盖客户端传入的 `taskId` / `globalTaskId`
- 生成 board 内 `#N`
- 生成全局 `G-N`
- 写入 `properties.__globalTaskId`

复制卡片：

- 生成新的 `#N`
- 生成新的 `G-N`
- 不沿用源卡片编号

删除/恢复卡片：

- 普通删除是 soft delete，卡片进入 `Deleted cards`，编号继续被历史占用
- 永久删除只移除卡片和恢复历史，不释放 board 内 `#N` 或全局 `G-N`
- 如果删除的是当前最高编号，下一张新卡片仍从历史最大编号继续递增，不复用这个最高编号
- 恢复已删除卡片时，通常保留原编号；如果遇到历史异常或冲突，服务端会重新分配不冲突的新编号
- UI 在 board 顶部三点菜单中增加 `Deleted cards` 入口，列出当前 board 已删除卡片并支持恢复或二次确认后永久删除

打开旧 board：

- 首次加载时迁移旧 `taskId`
- 首次加载当前 board 时迁移旧 `globalTaskId`
- 迁移最大值会同时参考未删除卡片和 deleted card history，避免重启或永久删除后遗忘历史最高号
- 迁移后按 board 使用进程内缓存，避免重复扫描

显示：

- 看板、表格、画廊、日历和卡片详情标题外侧显示全局 ID，格式从 `G-30392` 显示为 `#30392`
- 卡片详情属性区显示 `Board ID`，内容是 board 内数字编号，例如 `471`

## 验证命令

关键服务端测试：

```bash
cd server
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache go test ./app -run '^(TestCardTaskIDLifecycleUsesUniqueIDs|TestCreateCard|TestEnsureCardTaskIDs)$' -count=1
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache go test ./model -run '^(TestBlock2Card|TestCard2BlockIncludesTaskID)$' -count=1
```

前端检查：

```bash
cd webapp
npm run check
```

构建：

```bash
cd webapp
npm run pack

cd ../mattermost-plugin/webapp
npm run build

cd ../server
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache \
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 \
go build -trimpath -o dist/plugin-linux-loong64

cd ..
make bundle
```

包校验：

```bash
tar -xOzf mattermost-plugin/dist/focalboard-7.11.6.tar.gz focalboard/plugin.json
tar -tzf mattermost-plugin/dist/focalboard-7.11.6.tar.gz | grep plugin-linux
file mattermost-plugin/server/dist/plugin-linux-loong64
```

## 已知边界

- `taskId` 的严格唯一范围是单 board 的卡片生命周期编号；软删除和永久删除都不主动复用旧编号。
- `taskId` 的发号计数器持久化到 `system_settings`，永久删除后也不会因历史记录消失而回退。
- `globalTaskId` 的严格唯一范围是单插件进程串行写入场景下的卡片生命周期编号。
- 软删除和永久删除都不释放编号；编号空洞是为了保证代码引用和历史记录长期稳定。
- 如果同一个数据库有多个插件进程同时写入，严格跨进程唯一需要数据库事务序列或唯一约束。
- 当前 ID 存在 block fields JSON 中，不是独立索引列。
- 全局编号计数器持久化在 `system_settings`；插件重启后不会为了恢复最大值扫描全库。
- 如果系统设置里没有旧计数器，首次发号会扫描一次已有卡片并按最高正常 `G-N`
  初始化；不会再从当前毫秒时间戳起步。

## 产物

值得存档的插件包已放入仓库内 `release-archives/`，并随 git 提交保存。外层目录中的
`.zst` 副本已删除，只保留 Mattermost 可直接上传的 `.tar.gz` 包。

| 包位置 | 功能与用途 |
| --- | --- |
| `release-archives/focalboard-7.11.0-original-linux-loong64.tar.gz` | 官方 `v7.11.0` 原始行为，只做 loong64 构建/打包适配；用于测试回退到原版 Boards。 |
| `release-archives/focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz` | 只包含 board 内 `#N` 的可用回退版本。 |
| `release-archives/focalboard-7.11.0-taskid-linux-loong64.tar.gz` | 早期 board/global ID 构建，后续问题尚未全部修复。 |
| `release-archives/focalboard-7.11.1-taskid-linux-loong64.tar.gz` | 增加 manifest 版本号和 `Global ID` 属性显示。 |
| `release-archives/focalboard-7.11.2-taskid-linux-loong64.tar.gz` | 修复时间戳形态全局 ID，例如 `G-1780519393936`。 |
| `release-archives/focalboard-7.11.3-taskid-linux-loong64.tar.gz` | 删除最高编号后可复用编号；保留作历史对比，不建议用于长期代码引用。 |
| `release-archives/focalboard-7.11.4-taskid-linux-loong64.tar.gz` | 增加 `Deleted cards` 恢复弹窗。 |
| `release-archives/focalboard-7.11.5-taskid-linux-loong64.tar.gz` | 增加 `Deleted cards` 中的永久删除。 |
| `release-archives/focalboard-7.11.6-taskid-linux-loong64.tar.gz` | 外部显示全局 ID，详情显示 Board ID，修复 Deleted cards 显示/操作问题；仍存在部分恢复/永久删除路径会降低计数器的问题。 |
| `release-archives/focalboard-7.11.7-taskid-linux-loong64.tar.gz` | 当前推荐版本：修复恢复和永久删除边界路径会降低历史最大编号的问题，确保软删除和永久删除都不释放编号。 |
