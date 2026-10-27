import { useCallback, useEffect, useRef, useState } from "react";

import { edu, eduBlob, eduUpload, type EduScope } from "@/services/api/edu";

import { dragToSelection, fileHint, orderImages, selectionError, sizeText, type ProblemImage, type Selection } from "./problem-image-logic";
import { teacherError } from "./teacher-errors";

/**
 * 教师端：上传题目图片并框选单题（DEV-068）。服务端只接受 PNG/JPEG，校验内容与尺寸，
 * 重新编码以清除 EXIF/GPS 等元数据，不保存原文件名；图片只在教师视图出现，学生端不下发。
 * 图片需要登记来源与权利；关联题目时来源与题目已有来源必须一致。题面文字仍由老师录入核对。
 */
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";

type SourceRow = { id: string; title: string; state: string };

function useImageURL(scope: EduScope, id: string | null) {
    const [url, setUrl] = useState("");
    useEffect(() => {
        let live = true;
        let made = "";
        setUrl("");
        if (id)
            void eduBlob(`/problem-images/${id}/content`, scope).then((b) => {
                if (!live || !b) return;
                made = URL.createObjectURL(b);
                setUrl(made);
            });
        return () => {
            live = false;
            if (made) URL.revokeObjectURL(made);
        };
    }, [scope, id]);
    return url;
}

export function ProblemImagePanel({ scope, problemId }: { scope: EduScope; problemId?: string }) {
    const [sources, setSources] = useState<SourceRow[]>([]);
    const [source, setSource] = useState("");
    const [images, setImages] = useState<ProblemImage[]>([]);
    const [current, setCurrent] = useState<ProblemImage | null>(null);
    const [sel, setSel] = useState<Selection | null>(null);
    const [msg, setMsg] = useState("");
    const [busy, setBusy] = useState(false);
    const fileRef = useRef<HTMLInputElement>(null);
    const imgRef = useRef<HTMLImageElement>(null);
    const drag = useRef<{ x: number; y: number } | null>(null);
    const url = useImageURL(scope, current?.id || null);

    const load = useCallback(async () => {
        const r = await edu<{ images: ProblemImage[] }>("get", "/problem-images", scope);
        if (r.ok) setImages(orderImages(r.data.images || [], problemId));
    }, [scope, problemId]);
    useEffect(() => {
        void load();
        void edu<SourceRow[]>("get", "/source-assets", scope).then((r) => r.ok && setSources(r.data.filter((s) => s.state === "active")));
    }, [scope, load]);

    const upload = async () => {
        const f = fileRef.current?.files?.[0];
        if (!f) return setMsg("请选择图片文件");
        if (!source) return setMsg("请选择图片的来源与权利记录");
        const hint = fileHint(f);
        if (hint) return setMsg(hint);
        const form = new FormData();
        form.append("sourceAssetId", source);
        form.append("file", f);
        setBusy(true);
        const r = await eduUpload<ProblemImage>("/problem-images", scope, form);
        setBusy(false);
        if (!r.ok) return setMsg(teacherError(r));
        setMsg("已上传；服务端已清除图片元数据。请在图上拖动框选一道题，或使用整张图。");
        setCurrent(r.data);
        setSel(null);
        if (fileRef.current) fileRef.current.value = "";
        void load();
    };

    const point = (e: React.PointerEvent) => {
        const box = imgRef.current?.getBoundingClientRect();
        return box ? { x: e.clientX - box.left, y: e.clientY - box.top } : { x: 0, y: 0 };
    };
    const scale = () => (current && imgRef.current ? imgRef.current.clientWidth / current.width : 1);
    const onDown = (e: React.PointerEvent) => {
        drag.current = point(e);
    };
    const onMove = (e: React.PointerEvent) => {
        if (drag.current && current) setSel(dragToSelection(drag.current, point(e), scale(), current));
    };
    const onUp = () => {
        drag.current = null;
    };

    const select = async (whole: boolean) => {
        if (!current) return;
        if (!whole) {
            const e = sel ? selectionError(sel, current) : "请先在图上拖动框选一道题";
            if (e) return setMsg(e);
        }
        setBusy(true);
        const r = await edu<ProblemImage>("post", `/problem-images/${current.id}/select`, scope, whole ? { whole: true } : sel);
        setBusy(false);
        if (!r.ok) return setMsg(teacherError(r));
        setMsg(`已生成单题图（${r.data.width}×${r.data.height}）${problemId ? "，可以关联到本题" : "；保存题目后可关联"}`);
        setCurrent(r.data);
        setSel(null);
        void load();
    };

    const attach = async (img: ProblemImage) => {
        if (!problemId) return;
        const r = await edu<ProblemImage>("put", `/problems/${problemId}/image`, scope, { imageId: img.id });
        if (!r.ok) return setMsg(teacherError(r));
        setMsg("已关联到本题；题面文字仍以你录入并确认的内容为准。");
        void load();
    };
    const withdraw = async (img: ProblemImage) => {
        const r = await edu<{ changed: boolean }>("post", `/problem-images/${img.id}/withdraw`, scope);
        if (!r.ok) return setMsg(teacherError(r));
        if (current?.id === img.id) setCurrent(null);
        setMsg("已撤回，图片内容已删除");
        void load();
    };

    const k = scale();
    return (
        <details className="mt-2 rounded-md border border-dashed p-2">
            <summary className="cursor-pointer text-sm font-medium">题目图片（可选，仅教师可见）</summary>
            <p className="mt-1 text-xs text-stone-500">支持 PNG 或 JPEG，不超过 10 MB，每边 16 到 8192 像素。服务端会重新编码图片，清除拍摄位置等元数据，不保存文件名。系统不做图片识别，题面文字请在下方录入并核对。</p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
                <input ref={fileRef} aria-label="选择题目图片" type="file" accept="image/png,image/jpeg" className="text-xs" />
                <label htmlFor="pimg-src" className="text-xs">
                    来源与权利{" "}
                    <select id="pimg-src" className="h-8 rounded-md border px-1" value={source} onChange={(e) => setSource(e.target.value)}>
                        <option value="">请选择</option>
                        {sources.map((s) => (
                            <option key={s.id} value={s.id}>
                                {s.title}
                            </option>
                        ))}
                    </select>
                </label>
                <button type="button" className={btn} disabled={busy} onClick={() => void upload()}>
                    上传
                </button>
            </div>
            {sources.length === 0 ? <p className="mt-1 text-xs text-amber-700">本工作区还没有有效的来源记录，请先在来源管理中登记。</p> : null}
            {msg ? (
                <p role="status" aria-live="polite" className="mt-1 text-xs">
                    {msg}
                </p>
            ) : null}
            {current && url ? (
                <div className="mt-2">
                    <p className="text-xs text-stone-500">
                        {current.kind === "original" ? "原图" : "单题图"} {current.width}×{current.height}，{sizeText(current.bytes)}
                    </p>
                    <div className="relative inline-block max-w-full touch-none select-none" onPointerDown={current.kind === "original" ? onDown : undefined} onPointerMove={onMove} onPointerUp={onUp} onPointerLeave={onUp}>
                        <img ref={imgRef} src={url} alt={current.kind === "original" ? "上传的题目原图，拖动框选一道题" : "单题图"} className="max-h-96 max-w-full" draggable={false} />
                        {sel ? <div aria-hidden className="absolute border-2 border-sky-500 bg-sky-500/10" style={{ left: sel.x * k, top: sel.y * k, width: sel.w * k, height: sel.h * k }} /> : null}
                    </div>
                    {current.kind === "original" ? (
                        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
                            {(["x", "y", "w", "h"] as const).map((f) => (
                                <label key={f} htmlFor={`pimg-${f}`}>
                                    {f}{" "}
                                    <input
                                        id={`pimg-${f}`}
                                        type="number"
                                        min={0}
                                        className="h-7 w-20 rounded-md border px-1"
                                        value={sel ? sel[f] : ""}
                                        onChange={(e) => setSel({ ...(sel || { x: 0, y: 0, w: current.width, h: current.height }), [f]: Number(e.target.value) })}
                                    />
                                </label>
                            ))}
                            <button type="button" className={btn} disabled={busy} onClick={() => void select(false)}>
                                生成单题图
                            </button>
                            <button type="button" className={ghost} disabled={busy} onClick={() => void select(true)}>
                                使用整张图
                            </button>
                        </div>
                    ) : null}
                </div>
            ) : null}
            {images.length ? (
                <ul className="mt-2 space-y-1 text-xs">
                    {images.map((img) => (
                        <li key={img.id} className="flex flex-wrap items-center gap-2">
                            <button type="button" className="underline" onClick={() => setCurrent(img)}>
                                {img.kind === "original" ? "原图" : "单题图"} {img.width}×{img.height}
                            </button>
                            {problemId && img.problemId === problemId ? <span className="text-emerald-700">已关联本题</span> : null}
                            {img.kind === "single" && problemId && img.problemId !== problemId ? (
                                <button type="button" className={ghost} onClick={() => void attach(img)}>
                                    关联到本题
                                </button>
                            ) : null}
                            <button type="button" className={ghost} onClick={() => void withdraw(img)}>
                                撤回
                            </button>
                        </li>
                    ))}
                </ul>
            ) : null}
            <p className="mt-1 text-xs text-stone-500">未关联题目的图片按机构的临时文件保留期自动删除。</p>
        </details>
    );
}
