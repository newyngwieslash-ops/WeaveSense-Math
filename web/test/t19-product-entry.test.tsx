import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { buildNav, WorkspaceSidebarNav } from "@/components/layout/workspace-sidebar-nav";
import { CanvasFreeformEmptyState } from "@/components/canvas/canvas-short-drama-entry";
import { resolveAddNodeMenuCommands, type AddNodeMenuContext } from "@/lib/canvas/tool-registry";
import { defaultFeatureAvailability, useUserStore } from "@/stores/use-user-store";
import { CanvasNodeType } from "@/types/canvas";

const source = (file: string) => readFileSync(new URL(`../src/${file}`, import.meta.url), "utf8");
const noop = () => {};

describe("T19 product entry", () => {
    test("root redirects to intelligence and legacy routes share the short-drama gate", () => {
        const router = source("router.tsx");
        expect(router).toMatch(/path: "\/", element: <Navigate to="\/intelligence" replace \/>/);
        for (const path of ["create", "inspirations", "projects"]) {
            const route = router.slice(router.indexOf(`path: "/${path}"`)).split("},")[0];
            expect(route).toContain('<RequireFeature feature="shortDramaEnabled">');
        }
    });

    test("default navigation has five product entries and no legacy entry", () => {
        useUserStore.getState().setFeatures(defaultFeatureAvailability);
        const html = renderToStaticMarkup(<MemoryRouter initialEntries={["/intelligence"]}><WorkspaceSidebarNav collapsed={false} onNavigate={noop} onOpenSearch={noop} onExpand={noop} onCollapse={noop} /></MemoryRouter>);
        for (const label of ["情报中心", "研判画布", "行业包与技能", "数据资产", "任务记录", "情报与研判工作台"]) expect(html).toContain(label);
        for (const path of ["/create", "/projects", "/inspirations"]) expect(html).not.toContain(`href="${path}"`);
        expect(html).toContain('href="/intelligence/packs"');
        expect(html).toContain('href="/skills"');
    });

    test("explicit enablement restores legacy navigation", () => {
        const nav = buildNav({ ...defaultFeatureAvailability, shortDramaEnabled: true });
        const paths = nav.groups.flatMap(group => group.items.map(item => item.to));
        for (const path of ["/create", "/projects", "/inspirations"]) expect(paths).toContain(path);
        expect(buildNav(defaultFeatureAvailability).groups.flatMap(group => group.items).map(item => item.title)).toEqual([
            "情报中心", "研判画布", "行业包与技能", "数据资产", "任务记录",
        ]);
    });

    test("menus default closed while retaining analysis nodes and explicit enablement", () => {
        const legacy = ["style", CanvasNodeType.Script, "project-character", CanvasNodeType.BatchTable, "previs"];
        for (const shortDramaEnabled of [undefined, false, true]) {
            const ids = resolveAddNodeMenuCommands({ workspaceMode: "professional", isProjectLinked: false, shortDramaEnabled, handlers: {} as AddNodeMenuContext["handlers"] }).map(c => c.id);
            for (const id of legacy) expect(ids.includes(id)).toBe(shortDramaEnabled === true);
            for (const id of [CanvasNodeType.DataSource, CanvasNodeType.Logic, CanvasNodeType.Chart, CanvasNodeType.Markdown]) expect(ids).toContain(id);
        }
        const context = source("components/canvas/canvas-context-menu.tsx");
        expect(context).toContain("useCanvasCreateCommands(createContext)");
        expect(context).not.toContain("resolveAddNodeMenuCommands(createContext)");
    });

    test("empty canvas exposes real Agent, data-source import and topic flow actions", () => {
        const html = renderToStaticMarkup(<CanvasFreeformEmptyState commands={[]} onOpenAgent={noop} onImportLedger={noop} onOpenTopics={noop} />);
        for (const label of ["Agent 建模", "台账导入", "专题流"]) expect(html).toContain(label);
        const page = source("pages/canvas/project.tsx");
        expect(page).toContain("onOpenAgent={openAgent}");
        expect(page).toContain("onImportLedger={() => createNode(CanvasNodeType.DataSource)}");
        expect(page).toContain('onOpenTopics={() => navigate("/intelligence/topics")}');
    });

    test("handoff fallback and guidance no longer promise storyboards", () => {
        const handoff = source("pages/intelligence/canvas-handoff-modal.tsx");
        expect(handoff).not.toMatch(/分镜|视觉生成|视觉画面|预填创作/);
        expect(handoff).toContain("继续研判");
    });
});
