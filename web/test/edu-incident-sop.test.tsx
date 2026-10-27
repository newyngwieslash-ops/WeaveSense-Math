import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { todoLines, type HomeView } from "../src/components/edu/home-logic";
import { NOTICE_STATE_LABEL, noticeLine, reviewInputError, type RevisionNotice } from "../src/components/edu/incident-sop-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");
const notice = (state: string): RevisionNotice => ({
    notice: { id: "ern_1", incidentId: "eci_1", assignmentId: "eas_1", classId: "ecl_1", fromPublicationId: "epu_old", fixPublicationId: "epu_fix", state: "unread", createdAt: "2026-10-28T00:00:00Z" },
    assignmentTitle: "线面角",
    assignmentState: state,
    severity: "blocking",
    reason: "答案错误",
});
const ok = { rootCause: "投影取错底面", detection: "教师反馈", correctiveActions: "修正投影并补测", followUps: "" };

describe("DEV-126 incident SOP", () => {
    test("notice line names the task, the reason, its state and the fix", () => {
        const l = noticeLine(notice("suspended"));
        expect(l).toContain("线面角");
        expect(l).toContain("答案错误");
        expect(l).toContain("已暂停");
        expect(l).toContain("epu_fix");
        expect(noticeLine(notice("active"))).toContain("旧版本");
        expect(Object.keys(NOTICE_STATE_LABEL).sort()).toEqual(["acknowledged", "migrated", "unread"]);
    });
    test("home todo counts unread revision notices", () => {
        const v = { openIncidents: 0, urgentHelp: 0, pendingReview: [], openHelp: 0, revisionNotices: 2 } as unknown as HomeView;
        expect(todoLines(v).join()).toContain("2 条修订通知未读");
        expect(todoLines({ ...v, revisionNotices: 0 })).toEqual([]);
        expect(todoLines({ ...v, revisionNotices: undefined })).toEqual([]);
    });
    test("review input mirrors the server limits and refuses contact details", () => {
        expect(reviewInputError(ok)).toBe("");
        expect(reviewInputError({ ...ok, rootCause: "短" })).toContain("根因");
        expect(reviewInputError({ ...ok, detection: "" })).toContain("发现途径");
        expect(reviewInputError({ ...ok, correctiveActions: "x".repeat(801) })).toContain("纠正措施");
        expect(reviewInputError({ ...ok, followUps: "x".repeat(801) })).toContain("后续事项");
        expect(reviewInputError({ ...ok, detection: "联系 a.b@example.com" })).toContain("邮箱");
        expect(reviewInputError({ ...ok, detection: "电话13800138000" })).toContain("手机号");
    });
    test("server codes have teacher wording", () => {
        for (const code of ["INCIDENT_OPEN", "REVIEW_EXISTS", "PERSONAL_INFO"]) {
            expect(teacherError({ status: 409, code })).not.toContain("错误代码");
        }
    });
    test("panels are mounted: notices on home, reviews in admin", () => {
        expect(src("src/pages/edu/home.tsx")).toContain("<RevisionNoticePanel");
        expect(src("src/pages/edu/admin.tsx")).toContain("<IncidentReviewPanel");
        const c = src("src/components/edu/incident-sop.tsx");
        expect(c).toContain("/revision-notices?state=unread");
        expect(c).toContain("/incidents?reviewPending=1");
        expect(c).toContain("不可修改");
    });
});
