import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";

import { authorLink, JOB_STATUS_LABEL, LESSON_STATE_LABEL, todoLines, type HomeView } from "@/components/edu/home-logic";
import { edu } from "@/services/api/edu";
import { listIntelWorkspaces, type IntelWorkspace } from "@/services/api/intelligence";

/**
 * 教师首页（DEV-067）：我的讲解、待我审核、我的班级、学生问题、运行中的任务与内容事故。
 * 首次使用时提供原创示例题（明确标为示例，不计入使用与经营指标）。数据全部来自服务端按任课范围裁剪的 /home。
 */
const box = "rounded-lg border border-stone-200 p-4 dark:border-stone-700";
const btn = "inline-flex h-9 items-center rounded-md bg-stone-900 px-3 text-sm text-white dark:bg-stone-100 dark:text-stone-900";

export default function EduTeacherHome() {
    const [workspaces, setWorkspaces] = useState<IntelWorkspace[]>([]);
    const [ws, setWs] = useState("");
    const scope = useMemo(() => ({ workspace: ws }), [ws]);
    const [view, setView] = useState<HomeView | null>(null);
    const [msg, setMsg] = useState("");

    useEffect(() => {
        void listIntelWorkspaces().then((l) => {
            setWorkspaces(l);
            setWs(l[0]?.id || "");
        });
    }, []);
    useEffect(() => {
        if (!ws) return;
        void edu<HomeView>("get", "/home", scope).then((r) => {
            if (!r.ok) {
                setView(null);
                return setMsg(r.status === 403 ? "你在此工作区没有教师、审核或管理角色。" : `${r.code}：${r.message}`);
            }
            setMsg("");
            setView(r.data);
        });
    }, [ws, scope]);

    return (
        <main className="mx-auto max-w-5xl p-6">
            <div className="mb-4 flex flex-wrap items-center gap-3">
                <h1 className="text-xl font-semibold">教研工作台</h1>
                <select aria-label="工作区" className="h-9 rounded-md border px-2" value={ws} onChange={(e) => setWs(e.target.value)}>
                    {workspaces.map((w) => (
                        <option key={w.id} value={w.id}>
                            {w.name}
                        </option>
                    ))}
                </select>
                <Link className="text-sm underline" to="/edu/author">
                    备课
                </Link>
                <Link className="text-sm underline" to="/edu/teach">
                    班级与学情
                </Link>
            </div>
            {msg ? <p className="text-sm text-red-600">{msg}</p> : null}
            {view ? (
                <div className="space-y-4">
                    {view.firstRun ? (
                        <section className={`${box} border-amber-300`}>
                            <p className="text-xs font-medium text-amber-700">示例</p>
                            <h2 className="font-semibold">{view.sample.title}</h2>
                            <p className="mt-1 text-sm text-stone-600">{view.sample.note}。从录入、确认题意、分析、生成讲解、审核到发布走一遍完整流程。</p>
                            <Link className={`${btn} mt-2`} to={authorLink(ws, { sample: view.sample.familyId })}>
                                用示例题开始
                            </Link>
                        </section>
                    ) : null}
                    <section className={box}>
                        <h2 className="mb-2 font-semibold">待办</h2>
                        {todoLines(view).length ? (
                            <ul className="ml-4 list-disc text-sm">
                                {todoLines(view).map((l) => (
                                    <li key={l}>{l}</li>
                                ))}
                            </ul>
                        ) : (
                            <p className="text-sm text-stone-500">没有待办。</p>
                        )}
                    </section>
                    <section className={box}>
                        <h2 className="mb-2 font-semibold">待我审核（{view.reviewMode === "second_reviewer" ? "须他人审核" : "可自审"}）</h2>
                        {view.pendingReview.length ? (
                            <ul className="text-sm">
                                {view.pendingReview.map((l) => (
                                    <li key={l.id}>
                                        <Link className="underline" to={authorLink(ws, { lesson: l.id })}>
                                            {l.title || l.familyId}
                                        </Link>{" "}
                                        {new Date(l.createdAt).toLocaleString()}
                                    </li>
                                ))}
                            </ul>
                        ) : (
                            <p className="text-sm text-stone-500">暂无。</p>
                        )}
                    </section>
                    <section className={box}>
                        <h2 className="mb-2 font-semibold">我的讲解</h2>
                        {view.myLessons.length ? (
                            <ul className="text-sm">
                                {view.myLessons.map((l) => (
                                    <li key={l.id}>
                                        <Link className="underline" to={authorLink(ws, { lesson: l.id })}>
                                            {l.title || l.familyId}
                                        </Link>{" "}
                                        <span className="text-stone-500">{LESSON_STATE_LABEL[l.state] || l.state}</span>
                                    </li>
                                ))}
                            </ul>
                        ) : (
                            <p className="text-sm text-stone-500">还没有讲解。</p>
                        )}
                    </section>
                    <section className={box}>
                        <h2 className="mb-2 font-semibold">我的班级</h2>
                        {view.classes.length ? (
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="text-left text-stone-500">
                                        <th>班级</th>
                                        <th>学生</th>
                                        <th>进行中任务</th>
                                        <th>未回复问题</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {view.classes.map((k) => (
                                        <tr key={k.id}>
                                            <td>{k.name}</td>
                                            <td>{k.students}</td>
                                            <td>{k.activeAssignments}</td>
                                            <td>
                                                {k.openHelp}
                                                {k.urgentHelp ? `（其中 ${k.urgentHelp} 条报错）` : ""}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        ) : (
                            <p className="text-sm text-stone-500">还没有班级。</p>
                        )}
                    </section>
                    <section className={box}>
                        <h2 className="mb-2 font-semibold">运行中的任务</h2>
                        {view.runningJobs.length ? (
                            <ul className="text-sm">
                                {view.runningJobs.map((j) => (
                                    <li key={j.id}>
                                        {j.kind} {JOB_STATUS_LABEL[j.status] || j.status}
                                    </li>
                                ))}
                            </ul>
                        ) : (
                            <p className="text-sm text-stone-500">没有运行中的任务。</p>
                        )}
                    </section>
                </div>
            ) : null}
        </main>
    );
}
