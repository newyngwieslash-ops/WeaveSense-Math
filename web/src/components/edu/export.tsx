import { edu, type EduScope } from "@/services/api/edu";
import { teacherError } from "@/components/edu/teacher-errors";
import { EXPORT_LABEL, EXPORT_NOTE, saveHtml, type ExportFile } from "@/components/edu/export-logic";

const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";

async function run(path: string, scope: EduScope, onMsg: (m: string) => void, done: string) {
    const r = await edu<ExportFile>("get", path, scope);
    if (!r.ok) return onMsg(teacherError(r));
    saveHtml(r.data);
    onMsg(done);
}

/** 导出已发布讲解（DEV-103）：默认学生版，教师版需单独点击。 */
export function ContentExport({ publicationId, scope, onMsg }: { publicationId: string; scope: EduScope; onMsg: (m: string) => void }) {
    return (
        <div className="flex flex-wrap items-center gap-2 text-sm">
            <button className={ghost} onClick={() => void run(`/publications/${publicationId}/export`, scope, onMsg, `已导出学生版。${EXPORT_NOTE.student}`)}>
                {EXPORT_LABEL.student}
            </button>
            <button className={ghost} onClick={() => void run(`/publications/${publicationId}/export?audience=teacher`, scope, onMsg, `已导出教师版。${EXPORT_NOTE.teacher}`)}>
                {EXPORT_LABEL.teacher}
            </button>
            <span className="text-xs text-stone-500">导出时复核来源权利，文件附版本与审核清单。</span>
        </div>
    );
}

/** 导出一次作业的学习报告，只含班级汇总。 */
export function ReportExport({ assignmentId, scope, onMsg }: { assignmentId: string; scope: EduScope; onMsg: (m: string) => void }) {
    return (
        <button className={ghost} onClick={() => void run(`/assignments/${assignmentId}/report-export`, scope, onMsg, `已导出学习报告。${EXPORT_NOTE.report}`)}>
            {EXPORT_LABEL.report}
        </button>
    );
}
