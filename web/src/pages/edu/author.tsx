import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";

import { Classroom } from "@/components/edu/classroom";
import { LessonEditor } from "@/components/edu/lesson-editor";
import { ReviewChecklist } from "@/components/edu/review-checklist";
import { VariantSetPanel } from "@/components/edu/variant-set";
import { edu, newClientKey, type EduScope } from "@/services/api/edu";
import { listIntelWorkspaces, type IntelWorkspace } from "@/services/api/intelligence";
import { blockingCodeLabel } from "@/components/edu/render-check-logic";
import { ManualLessonForm } from "@/components/edu/manual-lesson";
import { TextIntake } from "@/components/edu/text-intake";
import { MANUAL_NOTICE_TEACHER, MANUAL_TEMPLATE_FAMILY, MANUAL_UNVERIFIED } from "@/components/edu/manual-lesson-logic";
import { JOB_STATE_TEXT, jobErrorText, lessonStateLine, STAGE_TEXT, teacherError } from "@/components/edu/teacher-errors";
import { ContentExport } from "@/components/edu/export";

/**
 * 教师备课：结构化录入 → 确认题意（绑定输入哈希）→ 数学分析任务 → 生成讲解任务 →
 * 审核（绑定讲解修订与哈希）→ 发布（不可变）→ 布置到班级。
 * 长任务只返回任务号并轮询；重试沿用同一个幂等键，未知结果交由服务端对账，不在前端盲目重发。
 */
const box = "rounded-lg border border-stone-200 p-4 dark:border-stone-700";
const btn = "inline-flex h-9 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-9 items-center rounded-md border border-stone-300 px-3 text-sm disabled:opacity-40 dark:border-stone-600";

export const FAMILY_TEMPLATES: Record<string, { label: string; structured: Record<string, unknown> }> = {
    "SOLID-ANGLE-02": { label: "正四棱锥侧棱与底面所成角", structured: { family_id: "SOLID-ANGLE-02", input_state: "sufficient", unit: "", params: { a: "2", h: "3" } } },
    "SOLID-ANGLE-01": { label: "正方体线面角", structured: { family_id: "SOLID-ANGLE-01", input_state: "sufficient", params: { e: "2" } } },
    "SOLID-VOL-01": {
        label: "长方体体积与单位换算",
        structured: { family_id: "SOLID-VOL-01", input_state: "sufficient", params: { l: "3", w: "20", h: "5" }, units: { l: "cm", w: "mm", h: "cm" }, target_unit: "cm" },
    },
    "CONIC-CHORD-01": { label: "椭圆焦点弦长范围", structured: { family_id: "CONIC-CHORD-01", input_state: "sufficient", params: { a: "2", b: "sqrt(3)" } } },
    "CONIC-SLOPE-01": { label: "椭圆中点弦斜率积", structured: { family_id: "CONIC-SLOPE-01", input_state: "sufficient", params: { a: "2", b: "sqrt(3)" } } },
    "CONIC-PARABOLA-01": { label: "抛物线焦点弦数量积", structured: { family_id: "CONIC-PARABOLA-01", input_state: "sufficient", params: { p: "2" } } },
    // 目录外题目：自动分析会返回 UNSUPPORTED_FAMILY，由老师人工编写讲解（DEV-039）。
    [MANUAL_TEMPLATE_FAMILY]: { label: "目录外题目（人工编写讲解）", structured: { family_id: MANUAL_TEMPLATE_FAMILY, input_state: "sufficient", params: {} } },
};

type Job = { id: string; status: string; stage: string; progress: number; error?: { code?: string; message?: string } | null; solutionRevisionId?: string; lessonRevisionId?: string };
type Revision = { id: string; problemId: string; inputHash: string; state: string; revision: number };
type Solution = { id: string; solutionStatus: string; mathState: string; evidenceLevels: string };
type Lesson = { id: string; lessonHash: string; state: string; blockingErrors: string; previewArtifactId: string };
const TERMINAL = new Set(["succeeded", "failed", "cancelled"]);

function useJob(scope: EduScope, jobId: string) {
    const [job, setJob] = useState<Job | null>(null);
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => {
        if (!jobId) return;
        let stop = false;
        const tick = async () => {
            const r = await edu<Job>("get", `/jobs/${jobId}`, scope);
            if (stop) return;
            if (r.ok) setJob(r.data);
            if (!r.ok || !TERMINAL.has(r.data.status)) timer.current = window.setTimeout(tick, 1500);
        };
        void tick();
        return () => {
            stop = true;
            window.clearTimeout(timer.current);
        };
    }, [jobId, scope]);
    return job;
}

export default function TeacherAuthoring() {
    const [workspaces, setWorkspaces] = useState<IntelWorkspace[]>([]);
    const [params] = useSearchParams();
    const [classroom, setClassroom] = useState(false);
    const [ws, setWs] = useState("");
    const scope = useMemo(() => ({ workspace: ws }), [ws]);
    const [family, setFamily] = useState("SOLID-ANGLE-02");
    const [title, setTitle] = useState("");
    const [json, setJson] = useState(JSON.stringify(FAMILY_TEMPLATES["SOLID-ANGLE-02"].structured, null, 2));
    const [rev, setRev] = useState<Revision | null>(null);
    const [analyzeKey] = useState(newClientKey);
    const [lessonKey] = useState(newClientKey);
    const [analyzeJob, setAnalyzeJob] = useState("");
    const [lessonJob, setLessonJob] = useState("");
    const [solution, setSolution] = useState<Solution | null>(null);
    const [lesson, setLesson] = useState<Lesson | null>(null);
    const [preview, setPreview] = useState("");
    const [publicationId, setPublicationId] = useState("");
    const [classes, setClasses] = useState<{ id: string; name: string }[]>([]);
    const [assign, setAssign] = useState({ classId: "", title: "", dueAt: "", answerPolicy: "teacher_release", unitPolicy: "required", approxAllowed: false, allowLate: false });
    const [msg, setMsg] = useState("");
    const [pubs, setPubs] = useState<{ id: string; lessonRevisionId: string; title: string; familyId: string; createdAt: string }[]>([]);
    const [variantSetId, setVariantSetId] = useState("");
    const [manual, setManual] = useState(false);
    const aj = useJob(scope, analyzeJob);
    const lj = useJob(scope, lessonJob);

    useEffect(() => {
        void listIntelWorkspaces().then((l) => {
            setWorkspaces(l);
            const want = params.get("ws");
            setWs(want && l.some((w) => w.id === want) ? want : l[0]?.id || "");
        });
    }, [params]);
    useEffect(() => {
        if (!ws) return;
        void edu<{ id: string; name: string }[]>("get", "/classes", scope).then((r) => r.ok && setClasses(r.data));
        void edu<typeof pubs>("get", "/publications", scope).then((r) => r.ok && setPubs(r.data));
    }, [ws, scope, publicationId]);
    // The variant set must come from the same lesson revision as the publication being assigned.
    const variantLesson = pubs.find((p) => p.id === publicationId)?.lessonRevisionId || lesson?.id || "";
    useEffect(() => {
        if (aj?.solutionRevisionId) void edu<Solution>("get", `/solution-revisions/${aj.solutionRevisionId}`, scope).then((r) => r.ok && setSolution(r.data));
    }, [aj?.solutionRevisionId, scope]);
    const openLesson = useCallback(
        (id: string) => {
            void edu<{ lesson: Lesson; publication?: { id: string }; quality?: { math?: { state?: string } } }>("get", `/lesson-revisions/${id}`, scope).then(async (r) => {
                if (!r.ok) return;
                setLesson(r.data.lesson);
                setManual(r.data.quality?.math?.state === MANUAL_UNVERIFIED);
                setPublicationId(r.data.publication?.id || "");
                setPreview("");
                if (r.data.lesson.previewArtifactId) {
                    const u = await edu<{ url: string }>("get", `/artifacts/${r.data.lesson.previewArtifactId}/url`, scope);
                    if (u.ok) setPreview(u.data.url);
                }
            });
        },
        [scope],
    );
    useEffect(() => {
        if (lj?.lessonRevisionId) openLesson(lj.lessonRevisionId);
    }, [lj?.lessonRevisionId, openLesson]);
    // 从教师首页进入：?lesson= 打开指定讲解修订；?sample= 预填原创示例题（标题标明“示例”）。
    useEffect(() => {
        const id = params.get("lesson");
        if (ws && id) openLesson(id);
    }, [ws, params, openLesson]);
    useEffect(() => {
        const f = params.get("sample");
        if (f && FAMILY_TEMPLATES[f]) {
            setFamily(f);
            setJson(JSON.stringify(FAMILY_TEMPLATES[f].structured, null, 2));
            setTitle(`示例：${FAMILY_TEMPLATES[f].label}`);
        }
    }, [params]);

    const pickFamily = (f: string) => {
        setFamily(f);
        setJson(JSON.stringify(FAMILY_TEMPLATES[f].structured, null, 2));
    };
    const save = async () => {
        let structured: unknown;
        try {
            structured = JSON.parse(json);
        } catch {
            return setMsg("结构化输入不是合法 JSON");
        }
        if (rev) {
            const r = await edu<Revision>("post", `/problems/${rev.problemId}/revisions`, scope, { baseRevision: rev.revision, structured });
            if (!r.ok) return setMsg(teacherError(r));
            setRev(r.data);
        } else {
            const r = await edu<{ revision: Revision }>("post", "/problems", scope, { familyId: family, title: title || FAMILY_TEMPLATES[family].label, structured });
            if (!r.ok) return setMsg(teacherError(r));
            setRev(r.data.revision);
        }
        setAnalyzeJob("");
        setLessonJob("");
        setSolution(null);
        setLesson(null);
        setManual(false);
        setMsg("已保存修订，请核对后确认题意");
    };
    const confirmRev = async () => {
        if (!rev) return;
        const r = await edu<Revision>("post", `/problem-revisions/${rev.id}/confirm`, scope, { inputHash: rev.inputHash });
        if (!r.ok) return setMsg(teacherError(r));
        setRev({ ...rev, ...r.data });
    };
    const analyze = async () => {
        if (!rev) return;
        const r = await edu<Job>("post", `/problem-revisions/${rev.id}/analyze`, scope, { idempotencyKey: `${analyzeKey}-${rev.id}` });
        if (!r.ok) return setMsg(teacherError(r));
        setAnalyzeJob(r.data.id);
    };
    const buildLesson = async () => {
        if (!solution) return;
        const r = await edu<Job>("post", `/solution-revisions/${solution.id}/lesson`, scope, { idempotencyKey: `${lessonKey}-${solution.id}` });
        if (!r.ok) return setMsg(teacherError(r));
        setLessonJob(r.data.id);
    };
    const publish = async () => {
        if (!lesson) return;
        const r = await edu<{ id: string }>("post", `/lesson-revisions/${lesson.id}/publish`, scope, { lessonHash: lesson.lessonHash });
        if (!r.ok) return setMsg(teacherError(r));
        setPublicationId(r.data.id);
    };
    const createAssignment = async () => {
        const r = await edu<{ id: string }>("post", `/classes/${assign.classId}/assignments`, scope, {
            publicationId,
            title: assign.title || title || FAMILY_TEMPLATES[family].label,
            dueAt: assign.dueAt ? new Date(assign.dueAt).toISOString() : undefined,
            allowLate: assign.allowLate,
            answerPolicy: assign.answerPolicy,
            approxAllowed: assign.approxAllowed,
            unitPolicy: assign.unitPolicy,
            variantSetId: variantSetId || undefined,
        });
        setMsg(r.ok ? "已布置到班级" : teacherError(r));
    };
    const jobLine = (j: Job | null) =>
        j ? (
            <p className="mt-2 text-sm">
                任务 {j.id.slice(-8)}：{JOB_STATE_TEXT[j.status] || j.status} {j.stage && !TERMINAL.has(j.status) ? `· ${STAGE_TEXT[j.stage] || j.stage}` : ""}
                {j.status === "failed" ? `。${jobErrorText(j.error)}` : ""}
                {j.status === "reconciliation_required" ? "，供应商结果未知，等待服务端对账，不会自动重复计费" : ""}
                {!TERMINAL.has(j.status) ? (
                    <button className={`${ghost} ml-2 h-7`} onClick={() => void edu("post", `/jobs/${j.id}/cancel`, scope, {})}>
                        取消
                    </button>
                ) : null}
            </p>
        ) : null;

    const confirmed = rev?.state === "confirmed";
    return (
        <main className="mx-auto w-full max-w-3xl px-4 py-6 text-foreground">
            <h1 className="mb-4 text-xl font-semibold">备课</h1>
            {msg ? (
                <p role="status" aria-live="polite" className="mb-3 rounded-md bg-amber-50 p-2 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100">
                    {msg}
                </p>
            ) : null}
            <section className={box}>
                <h2 className="mb-2 font-semibold">1. 题目与参数</h2>
                <div className="flex flex-wrap gap-2">
                    <select aria-label="工作区" className="h-9 rounded-md border px-2" value={ws} onChange={(e) => setWs(e.target.value)}>
                        {workspaces.map((w) => (
                            <option key={w.id} value={w.id}>
                                {w.name}
                            </option>
                        ))}
                    </select>
                    <select aria-label="题型" className="h-9 rounded-md border px-2" value={family} disabled={!!rev} onChange={(e) => pickFamily(e.target.value)}>
                        {Object.entries(FAMILY_TEMPLATES).map(([k, v]) => (
                            <option key={k} value={k}>
                                {v.label}
                            </option>
                        ))}
                    </select>
                    <input aria-label="标题" className="h-9 flex-1 rounded-md border px-2" placeholder="标题" value={title} onChange={(e) => setTitle(e.target.value)} />
                </div>
                <TextIntake
                    scope={scope}
                    disabled={!!rev}
                    onDraft={(d) => {
                        if (FAMILY_TEMPLATES[d.family_id]) setFamily(d.family_id);
                        setJson(JSON.stringify(d.structured, null, 2));
                        if (!title) setTitle(d.summary.object);
                        setMsg("已从文字识别出草稿，请核对参数后保存并确认题意");
                    }}
                    onManual={() => pickFamily(MANUAL_TEMPLATE_FAMILY)}
                />
                <textarea aria-label="结构化题目输入（JSON）" className="mt-2 w-full rounded-md border p-2 font-mono text-xs" rows={8} value={json} onChange={(e) => setJson(e.target.value)} />
                <div className="mt-2 flex gap-2">
                    <button className={ghost} onClick={() => void save()}>
                        {rev ? "保存为新修订" : "保存"}
                    </button>
                    <button className={btn} disabled={!rev || confirmed} onClick={() => void confirmRev()}>
                        {confirmed ? "题意已确认" : "确认题意"}
                    </button>
                </div>
                {rev ? (
                    <p className="mt-1 text-xs text-stone-500">
                        修订 {rev.revision}，输入哈希 {rev.inputHash.slice(0, 12)}
                    </p>
                ) : null}
            </section>

            <section className={`${box} mt-4`}>
                <h2 className="mb-2 font-semibold">2. 数学分析</h2>
                <button className={btn} disabled={!confirmed || !!analyzeJob} onClick={() => void analyze()}>
                    开始分析
                </button>
                {jobLine(aj)}
                {solution ? (
                    <p className="mt-2 text-sm">
                        结论状态：{solution.solutionStatus}；数学状态：{solution.mathState}；证据层级：{solution.evidenceLevels}
                    </p>
                ) : null}
                {confirmed && rev ? (
                    <details className="mt-3" open={aj?.error?.code === "UNSUPPORTED_FAMILY" || family === MANUAL_TEMPLATE_FAMILY}>
                        <summary className="cursor-pointer text-sm font-medium">人工编写讲解（目录外题目）</summary>
                        <ManualLessonForm scope={scope} revisionId={rev.id} onJob={setLessonJob} />
                    </details>
                ) : null}
            </section>

            <section className={`${box} mt-4`}>
                <h2 className="mb-2 font-semibold">3. 讲解与审核</h2>
                <button className={btn} disabled={!solution || !!lessonJob} onClick={() => void buildLesson()}>
                    生成讲解
                </button>
                {jobLine(lj)}
                {lesson ? (
                    <>
                        <p className="mt-2 text-sm">
                            {lessonStateLine(lesson.state, lesson.blockingErrors, blockingCodeLabel)}，讲解哈希 {lesson.lessonHash.slice(0, 12)}
                        </p>
                        {preview ? <iframe title="教师预览" src={preview} sandbox="allow-scripts" className="mt-2 h-96 w-full rounded-md border" /> : null}
                        <ReviewChecklist key={lesson.id + lesson.state} scope={scope} lesson={lesson} onDone={(decision) => setLesson({ ...lesson, state: decision })} />
                        <div className="mt-2 flex gap-2">
                            <button className={btn} disabled={lesson.state !== "approved" || !!publicationId} onClick={() => void publish()}>
                                {publicationId ? "已发布" : "发布"}
                            </button>
                            <button className={btn} onClick={() => setClassroom(!classroom)}>
                                {classroom ? "关闭课堂演示" : "课堂演示"}
                            </button>
                        </div>
                        {classroom ? <Classroom key={lesson.id} scope={scope} lessonId={lesson.id} onClose={() => setClassroom(false)} /> : null}
                        {manual ? (
                            <p role="note" className="mt-2 rounded-md bg-stone-100 p-2 text-sm dark:bg-stone-800">
                                {MANUAL_NOTICE_TEACHER}
                            </p>
                        ) : null}
                        {lesson.blockingErrors || manual ? null : (
                            <details className="mt-3">
                                <summary className="cursor-pointer text-sm font-medium">修改讲解（生成新修订）</summary>
                                <div className="mt-2">
                                    <LessonEditor key={lesson.id} scope={scope} lessonId={lesson.id} onOpenRevision={openLesson} />
                                </div>
                            </details>
                        )}
                    </>
                ) : null}
            </section>

            <section className={`${box} mt-4`}>
                <h2 className="mb-2 font-semibold">4. 举一反三自测题组（可选）</h2>
                {manual && variantLesson === lesson?.id ? (
                    <p className="text-sm text-stone-500">人工编写的讲解不生成变式题组，也没有自测题。</p>
                ) : variantLesson ? (
                    <VariantSetPanel scope={scope} lessonId={variantLesson} picked={variantSetId} onPick={setVariantSetId} />
                ) : (
                    <p className="text-sm text-stone-500">先生成讲解或选择已发布的讲解。不选题组时，学生自测使用讲解自带的一道同构题。</p>
                )}
            </section>

            <section className={`${box} mt-4`}>
                <h2 className="mb-2 font-semibold">5. 布置到班级</h2>
                <div className="grid gap-2 text-sm md:grid-cols-2">
                    <select
                        className="h-9 rounded-md border px-2 md:col-span-2"
                        value={publicationId}
                        onChange={(e) => {
                            setPublicationId(e.target.value);
                            setVariantSetId("");
                        }}
                    >
                        <option value="">选择已发布的讲解</option>
                        {pubs.map((p) => (
                            <option key={p.id} value={p.id}>
                                {p.title || p.familyId}（{new Date(p.createdAt).toLocaleString()}）
                            </option>
                        ))}
                    </select>
                    {publicationId ? (
                        <div className="md:col-span-2">
                            <ContentExport publicationId={publicationId} scope={scope} onMsg={setMsg} />
                        </div>
                    ) : null}
                    {variantSetId ? <p className="md:col-span-2">自测使用已审核题组 {variantSetId.slice(-6)}，学生全部提交后才显示结果</p> : null}
                    <select className="h-9 rounded-md border px-2" value={assign.classId} onChange={(e) => setAssign({ ...assign, classId: e.target.value })}>
                        <option value="">选择班级</option>
                        {classes.map((c) => (
                            <option key={c.id} value={c.id}>
                                {c.name}
                            </option>
                        ))}
                    </select>
                    <input className="h-9 rounded-md border px-2" placeholder="作业标题" value={assign.title} onChange={(e) => setAssign({ ...assign, title: e.target.value })} />
                    <label>
                        截止时间 <input type="datetime-local" className="h-9 rounded-md border px-2" value={assign.dueAt} onChange={(e) => setAssign({ ...assign, dueAt: e.target.value })} />
                    </label>
                    <select className="h-9 rounded-md border px-2" value={assign.answerPolicy} onChange={(e) => setAssign({ ...assign, answerPolicy: e.target.value })}>
                        <option value="teacher_release">老师手动开放答案</option>
                        <option value="after_submission">学生提交后开放</option>
                        <option value="after_due">截止后开放</option>
                    </select>
                    <select className="h-9 rounded-md border px-2" value={assign.unitPolicy} onChange={(e) => setAssign({ ...assign, unitPolicy: e.target.value })}>
                        <option value="required">必须带单位</option>
                        <option value="optional">单位可省略</option>
                    </select>
                    <label>
                        <input type="checkbox" checked={assign.approxAllowed} onChange={(e) => setAssign({ ...assign, approxAllowed: e.target.checked })} /> 允许近似值
                    </label>
                    <label>
                        <input type="checkbox" checked={assign.allowLate} onChange={(e) => setAssign({ ...assign, allowLate: e.target.checked })} /> 允许迟交
                    </label>
                </div>
                <button className={`${btn} mt-2`} disabled={!publicationId || !assign.classId} onClick={() => void createAssignment()}>
                    布置
                </button>
            </section>
        </main>
    );
}
