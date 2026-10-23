# F03可信基线与保存衔接验证

2026-10-23，Asia/Shanghai。分支 `codex/fix-workspace-member-controls-and-analyst-permissions`，实现基线HEAD `8de18f8a445a75555498d88fd36a8432cef4da32`。Native候选2 `f5eddd25-b832-4544-b58b-7b0ab2ee9abd`，用户先明确接受无独立Verifier的有限结果，再独立确认本地归档；实际归档完成，stateVersion12/done/archived=true、工作区kept。Runtime六项按所接受限制通过，不代表独立审查、原F03恢复或全量通过；无模型/子代理/推送合并发布。

2026-10-23 13:01 +08:00归档补记：重新dry-run退出0、ready=true、无阻塞/无需重验；随后archive --confirmed退出0，新主规格已创建，工件移至 `docs/superpowers/comet/archive/2026-10-23-fix-canvas-pending-edit-sync`。current模式未自动创建提交，授权范围内的实现、测试、文档和归档工件另作精确暂存提交，最终哈希见最新交接。归档后status退出0，5项Native均done/archived，0活跃；没有重复产品测试。

诊断限制：归档后调用doctor同名参数退出65，称active change需要migration；原活动目录已不存在，status却正确解析archiveRef并显示done/stateVersion12。没有据此执行迁移或改写归档状态。正式结束依据实际archive返回、归档工件/主规格和status交叉核对，不宣称doctor通过。

2026-10-23 12:56 +08:00补记：本次只执行接受记录及归档dry-run（退出0、ready=true、blockers=[]、requiresReverification=false），未重复产品测试。预览将新增canvas-pending-edit-sync主规格并移动本变更工件到2026-10-23归档目录；尚未执行archive --confirmed，尚无归档或提交授权。

## 结论与实际边界

修复了两个可复现共享缺口：脏缓存重开无法利用精确匹配的旧云端哈希恢复可信基线；初始化将脏缓存当已确认导致直接保存返回成功却零PATCH。Agent刷新有同类基线判断。三个入口共同保留严格冲突/缺基线保护、原mutation重放和后续编辑，不改API/表结构/RLS/正则False语义。

**原F03历史图尚未收口。** `3zJp590stUfdwNLbRe4xF`仍本地`^深层甲$`、PG revision5/`*`；正常导出所选入口实际报“缺少可信画布合并基线，已保留本地编辑”，表明真实旧现场不满足本次精确匹配条件。没有清缓存、SQL/HTTP强写、猜测哈希或恢复覆盖。已证实的共享缺口不是全部历史现场根因的唯一证明，后续必须有可审阅且可验证的旧基线/差异证据。

## RED与实现

| 阶段 | 实际检查 | 结果 |
|---|---|---|
| 首次加载RED | `bun test test/canvas-pending-edit-sync.test.ts` | 正则/普通标题两例在PATCH前抛缺可信基线 |
| 同源Agent刷新RED | 同专项追加Agent刷新 | 8pass/1fail，失败于Agent缺合并基线 |
| 显式直存RED | `bun test test/canvas-pending-edit-sync.test.ts -t 显式直存` | 期望远端6实际5，无提交却成功返回 |
| 最终新增专项 | 同文件 | 16pass/0fail/79断言，包含自动保存、ACK未知重开原payload、提交中新编辑、确认读取失败、哈希不足/权限/账号及另一坏图隔离 |
| 最终Runtime专项 | 七文件，见Runtime日志 | 64pass/0fail/221断言，exit0，2728ms |
| Runtime构建 | `cd web; bun run build` | tsc/Vite exit0，18744ms |
| Runtime lint | `cd web; bun run lint` | exit0，38558ms |
| 全Web最终开发检查 | `cd web; bun test` | 2234pass/40fail/2errors，17314断言，61.55s；非GREEN |

Runtime检查日志在 `.comet/runtime/native/changes/fix-canvas-pending-edit-sync/logs/checks/`，候选2记录64/221；第一轮59/198已被后续候选替代。完整Web日志在 `.local/flow-uat-20261020/f03-sync/full-web-tests-final.log`。独立Verifier没有启动，Runtime记录unavailable而不是伪造独立PASS。

共享保存直接调用方为画布生命周期/列表/卡片/文件夹、素材库/上传/选择器、项目素材同步、previs、creation入口/对话、Agent保存/审批；共享加载还由生成消费者、任务结果同步调用。检索并读取相关上下文，复用同一锁内加载体，不逐调用方加绕过。初始缓存对自身的比较不能证明云端确认，因此不再登记为acknowledged；日志中已有可靠baseline仍沿用，pending原请求优先。

## 18088实际验证

只保卷更新web，最终镜像 `flow-uat-web:f03-sync-r2-20261023`，digest `sha256:5f0d87659cc03e4fcf0b548ae4d20eb6d25fefdf615d5b6ad4cb28b392b4a6e9`，页面实际entry `index-lcavFfza.js`与本次构建一致。含旧Agent面板dirty，非纯HEAD；8088及非web服务不动。第一轮打开旧tab刷新没有切换bundle，随后新标签核对新entry，不以旧页结果冒称候选结果。

新图通过真实ZIP选择器导入，ID `yys1O-fPw8d2fncnxd1sf`，6节点/5边、无媒体/模型或上游。真实输入`*`后PG revision3/`*`；恢复`^深层甲$`后页面True1/False3，PG revision4且合法值。尝试立即goto被浏览器ERR_ABORTED阻止，不能称立即离页通过；随后使用正常应用主页/画布库导航，新候选重开合法值/已同步保持。r2再修改节点名称，PG revision5，标题`F03 合法标题持久化`、正则保持，页面已同步。

实际截图 `.local/flow-uat-20261020/f03-sync/ui-final.png`。普通页面截图只证明所示DOM/UI，落库另由只读PG交叉核对。新图在线重开不等于真正离线、冷启动或独立Profile测试。

保护前后：画布16→17，仅新增1图；原16图整行hash均一致（包括原F03），10文档/1报告hash不变，模型任务/预算0不变，非web四容器ID/镜像/启动时间一致。before/after原文仅在忽略目录，不输出账号scope或凭据值。

## 未通过与操作限制

原F03缺可信基线仍阻塞；全Web旧40失败/2错误未修；独立Verifier未执行，用户已明确接受这一限制下的有限结果，并另行授权本地归档/相关提交。T4部分通过，不把新图通过覆盖原图失败。未知ACK与并发新编辑为真实共享函数+HTTP/存储替身检查，没有真实断网注入，不称live ACK故障演练。

本地导出副本曾等待超时320秒，未取得文件，停止该路径；后来“导出所选”明确显示缺基线错误。未重复清缓存/恢复尝试，未触碰原图编辑。保留原tab、失败证据和现有分支/修改。本次没有API/数据库变更；其他U/A、真实模型/实采外发/迁移及G01发布门槛不解除。

## 全Web原有失败明细

与上一预览覆盖报告对照：38个去重失败名称完全相同，old_only/new_only均空，40次失败；两异常为 `agent-api-reliability.test.ts` 临时复制模块无法解析`@/lib/user-scope`、`agent-launcher-position.test.mjs` Windows `/E:/...` BadPathName。以下逐名保留，不据名称推定40产品缺陷：

```
Canvas keyboard keeps node copy as the fallback when no text is selected
DEV previs lab loads without calling the appearance backend
a pending skills response cannot replace the cache after installation
added Skills retries a transient backend proxy failure
appearance loads in parallel with normal startup: dev=false path=/dev/previs-repro
appearance loads in parallel with normal startup: dev=false path=/login
appearance loads in parallel with normal startup: dev=true path=/dev/previs-repro-other
appearance loads in parallel with normal startup: dev=true path=/dev/previs-repro/
appearance loads in parallel with normal startup: dev=true path=/login
axios.create stays in the shared request client
business API modules call http instead of axios or request(apiClient)
canvas custom appearance > stores defaults locally with the active account scope
canvas resource mention editor > refreshes async reference previews even when prompt text has not changed
collection select focus uses a thin keyboard ring, not a persistent pointer outline
commit image reuse > only a missing manifest permits a rebuild
commit image reuse > reuses the exact existing digest
connection menu omits the header and provides descriptions for every option
digest promotion > current main promotes latest and SHA tags using digests
digest promotion > stale main cannot roll latest back
digest promotion > tag release promotes the exact version without touching latest
large canvas media rendering > decodes canvas images synchronously so zoom re-rasterization never paints blank frames
large canvas media rendering > keeps inactive video nodes on a viewport-gated static first frame
member revoke uses workspace header, encoded ID and rejects business failure
menu surfaces are explicitly scoped and old account inner overrides are removed
model quote API targets > direct system videos send the actual options and cancellation signal to the catalog quote
model quote API targets > logical models retain their dedicated quote route
public film entry remains independent: /welcome
public film entry remains independent: /welcome/
welcome entry loads configured appearance
传输结束不吞掉服务端保存错误
分片上传累加片内真实进度，传输 100% 后仍须等待合并响应
图片上传占位 > 上传先占位再传输，完成替换同一节点，弹窗不等待传输完成
普通 multipart 将传输进度换算为文件字节，总量未知不伪造百分比
资源直传失败分类 > 保留后端可读文案，并把 multipart 超限翻译成中文
资源直传失败分类 > 成功响应仍返回后端资源本身
资源直传失败分类 > 断网等非 HTTP 失败按瞬时处理
资源直传失败分类 > 鉴权、越权与体积超限属于重试不会自愈的永久失败
资源直传失败分类 > 限流与服务端故障属于瞬时失败，允许退回本机后重传
```
