/**
 * 教师端错误说明（DEV-083）。只按错误码与 HTTP 状态给出中文说明和下一步动作，
 * 从不显示服务端原始 message、堆栈或内部响应；未知错误只显示错误码，便于报告。
 */
type ErrLike = { status?: number; code?: string; offline?: boolean };

const BY_CODE: Record<string, [string, string]> = {
    NETWORK: ["网络不可用，本次操作没有提交", "检查网络后重试"],
    BUDGET_EXCEEDED: ["额度不足，计算没有提交，也没有扣费", "请机构管理员在额度设置中调整，或等下个周期再试"],
    RATE_LIMITED: ["操作太频繁，已暂时拒绝", "稍等一分钟后再试"],
    RIGHTS_RESTRICTED: ["来源权利不允许这样使用（已过期、已撤回，或未授予学生使用、改编或导出）", "在来源管理中更换或续期来源"],
    POLICY_REVOKED: ["机构策略或你的角色不允许这个操作", "请联系审核人或机构管理员"],
    CONSENT_REQUIRED: ["学生的必要同意缺失或已撤回", "请机构按流程补齐同意后再操作"],
    REVISION_CONFLICT: ["内容已被更新，当前页面不是最新修订", "刷新后在最新修订上继续"],
    VERSION_CONFLICT: ["设置已被他人修改", "刷新后重新修改并保存"],
    CONFLICT: ["与当前状态冲突，没有保存", "刷新查看最新状态后再操作"],
    IDEMPOTENCY_CONFLICT: ["同一个操作已经用不同内容提交过", "刷新后重新发起"],
    CHECKLIST_OPEN: ["审核清单还有未处理的问题或定位意见", "处理完问题项后再通过，或选择退回"],
    VALIDATION_FAILED: ["内容有阻断错误，不能继续", "按阻断说明修改后重新生成"],
    INPUT_AMBIGUOUS: ["输入不完整或有歧义", "补全题目条件并确认题意后再提交"],
    PARAMETER_LIMIT: ["参数超出支持范围", "调整到支持范围内后再提交"],
    UNSUPPORTED_FAMILY: ["这类题目目前不在支持范围内", "换用已支持的题型，或在“人工编写讲解”中自己写步骤和答案"],
    MANUAL_UNVERIFIED: ["这是老师人工编写的讲解，未经自动数学验证，不能自动判分、生成自测或变式", "由老师人工批改学生作答"],
    CONDITION_CONFLICT: ["题目条件互相矛盾或不成立", "检查条件后修改题目"],
    METHOD_MISMATCH: ["要求的解法与题目不匹配", "调整允许的方法后重试"],
    TIMEOUT_UNDETERMINED: ["计算超时，结果未确定（不代表无解）", "稍后重试，或简化条件"],
    PROVIDER_UNAVAILABLE: ["数学服务暂时不可用", "稍后重试；持续出现请联系管理员"],
    CLASS_ARCHIVED: ["班级已归档，不能再邀请学生、布置任务或修改", "在仍在使用的班级中操作；需要时新建班级"],
    ASSIGNMENT_CLOSED: ["任务已截止", "需要时为学生设置延期"],
    ASSIGNMENT_WITHDRAWN: ["任务已经撤回", "刷新任务列表；需要时重新布置"],
    ANSWER_NOT_RELEASED: ["答案尚未开放", "按任务的答案策略开放后再查看"],
    PERSONAL_INFO: ["内容中疑似包含学生姓名或联系方式", "删去个人信息后再提交"],
    DATA_REQUEST_OPEN: ["这名学生已有同类请求未处理完", "先完成或拒绝那一条，再登记新的请求"],
    DATA_REQUEST_NOT_VERIFIED: ["还没有记录申请人身份核验", "线下核验后填写核验单号，再预览和完成"],
    OPS_GRANT_INVALID: ["没有有效的运维授权，或授权范围不包括这项内容", "请学校管理员在组织管理中授权，或检查授权是否到期、已撤销"],
    DATA_REQUEST_CLOSED: ["这条请求已完成或已拒绝", "刷新列表；如需再次处理请重新登记"],
    RETENTION_NOT_CONFIRMED: ["数据保留期还是建议值，机构确认前不会清理任何数据", "在机构管理的数据保留一节核对各项天数，写明理由后确认"],
    ACCESS_PURPOSE_REQUIRED: ["查看非任课班级的个体学生记录需要说明业务目的", "填写 4 到 200 字的目的后再查看；每次查看都会记入访问记录"],
    FORBIDDEN: ["你没有这个操作的权限，或不在该工作区", "确认工作区与角色；需要时联系机构管理员"],
    NOT_FOUND: ["内容不存在、已撤回，或不在你的工作区", "返回列表刷新后重新选择"],
    GENERATION_PAUSED: ["新生成已被管理员暂停（平台或这类题目）", "已有内容和学生作答不受影响；等管理员解除暂停后再生成"],
    CONTENT_PAUSED: ["这份内容或这个发布版已被管理员暂停使用", "改用其他发布版；已布置的任务可迁移到仍有效的旧发布版"],
    PAUSE_CHECK_UNAVAILABLE: ["暂时无法确认这项内容是否被暂停，为安全起见本次操作没有执行", "稍后重试；持续出现请联系管理员"],
    INCIDENT_OPEN: ["事故还没有关闭，不能写复盘", "先发布经过重新审核的修正版并关闭事故"],
    REVIEW_EXISTS: ["这个事故已经写过复盘，复盘不可修改", "刷新后查看已有复盘；补充内容记入后续事项的新事故或工单"],
    MATH_SERVICE_UNAVAILABLE: ["数学服务暂时不可用", "稍后重试；持续出现请联系管理员"],
    MATH_TOOL_FAILED: ["数学计算没有得到可用结果", "检查题目条件后重试"],
    RECEIPT_INVALID: ["数学服务返回的结果没有通过平台校验，已丢弃", "请联系管理员核查，不要重复提交"],
    IDEMPOTENCY_REQUIRED: ["请求缺少防重复标识", "刷新页面后重试"],
    IMAGE_TOO_LARGE: ["图片超过 10 MB，没有上传", "压缩或裁小后再上传"],
    UNSUPPORTED_IMAGE: ["只支持 PNG 或 JPEG 图片", "把图片另存为 PNG 或 JPEG 后再上传"],
    IMAGE_TYPE_MISMATCH: ["文件内容与扩展名或类型不一致", "用看图软件重新另存为 PNG 或 JPEG 后再上传"],
    IMAGE_DECODE_FAILED: ["图片已损坏或无法读取", "重新截图或另存后再上传"],
    IMAGE_DIMENSIONS: ["图片尺寸超出范围（每边 16 到 8192 像素，总像素不超过 4000 万）", "缩放到范围内后再上传"],
    IMAGE_BUSY: ["图片处理繁忙，本次没有上传", "稍等片刻后再试"],
    IMAGE_NOT_SINGLE: ["原图不能直接关联题目", "先框选一道题生成单题图，再关联"],
    WITHDRAWN: ["内容已撤回，学生不再可见", "如需继续使用，请修改后生成新修订并重新审核发布"],
};

const BY_STATUS: Record<number, [string, string]> = {
    401: ["登录已失效", "重新登录后再试"],
    403: ["你没有这个操作的权限，或不在该工作区", "确认工作区与角色；需要时联系机构管理员"],
    404: ["内容不存在、已撤回，或不在你的工作区", "返回列表刷新后重新选择"],
    413: ["内容太大", "缩小后再提交"],
};

export function teacherErrorParts(e: ErrLike): { text: string; action: string; code: string } {
    const code = e.offline ? "NETWORK" : e.code || (e.status ? `HTTP_${e.status}` : "UNKNOWN");
    const hit = BY_CODE[code] || (e.status ? BY_STATUS[e.status] : undefined);
    if (hit) return { text: hit[0], action: hit[1], code };
    return { text: "操作没有完成", action: "稍后重试；持续出现请把错误代码告诉管理员", code };
}

/** 一句话说明加下一步，未知错误附错误码；从不包含服务端原文。 */
export function teacherError(e: ErrLike): string {
    const p = teacherErrorParts(e);
    const known = !!BY_CODE[p.code] || (e.status ? !!BY_STATUS[e.status] : false);
    return `${p.text}。下一步：${p.action}${known ? "" : `（错误代码 ${p.code}）`}`;
}

/** 作业失败：只用错误码。 */
export function jobErrorText(err: { code?: string } | null | undefined): string {
    if (!err?.code) return "作业失败。下一步：稍后重试；持续出现请联系管理员";
    return teacherError({ code: err.code });
}

/**
 * 课堂与学情页的空态（DEV-083）：每种空态都给出下一步。数量为 -1 表示尚未载入，不显示空态。
 */
export function emptyState(v: { classes: number; classId: string; assignments: number; asId: string; attempts: number }): string {
    if (v.classes === 0) return "还没有班级。下一步：在下方“班级、邀请码与名册”中新建班级，再把邀请码发给学生";
    if (!v.classId) return "";
    if (v.assignments === 0) return "这个班级还没有作业。下一步：在备课页把已审核发布的讲解布置到这个班级";
    if (!v.asId) return "";
    if (v.attempts === 0) return "这项作业还没有学生作答。学生开始作答后，学情分布与作答记录会显示在这里";
    return "";
}

export const JOB_STATE_TEXT: Record<string, string> = {
    queued: "排队中",
    running: "运行中",
    needs_confirmation: "等待确认",
    succeeded: "已完成",
    failed: "失败",
    cancelled: "已取消",
    reconciliation_required: "等待对账",
};
export const STAGE_TEXT: Record<string, string> = {
    validate_input: "校验输入",
    solve: "求解",
    verify: "独立验证",
    compose_lesson: "生成讲解",
    project_student: "生成学生视图",
    render_preview: "渲染预览",
    check_render: "呈现检查",
    generate_variants: "生成变式",
    verify_variants: "验证变式",
    apply_edits: "应用修改",
};
export const LESSON_STATE_TEXT: Record<string, string> = {
    ready_for_review: "待审核",
    approved: "已通过",
    rejected: "已退回或被阻断",
    superseded: "已被新修订取代",
    withdrawn: "已撤回，学生不可见",
};

/** 讲解修订状态一句话；有阻断错误时说明原因与下一步。 */
export function lessonStateLine(state: string, blockingErrors: string, label: (code: string) => string): string {
    const base = LESSON_STATE_TEXT[state] || "状态未知";
    let codes: string[] = [];
    try {
        const v = JSON.parse(blockingErrors || "[]");
        codes = Array.isArray(v) ? v.map(String) : [];
    } catch {
        codes = blockingErrors ? ["unknown"] : [];
    }
    if (!codes.length) return base;
    return `${base}；阻断错误：${codes.map(label).join("；")}。不可发布，下一步：修改题目或讲解后重新生成`;
}
