import { describe, expect, it } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { BOARD_FONT_FACE, BOARD_FONT_FAMILY, BOARD_FONT_PACKAGE, BOARD_FONT_VERSION, BOARD_THEME_VERSION } from "../src/components/edu/board-theme";

// DEV-072：板书字体随应用打包、精确锁定版本与完整性 hash，不从外部加载，不引入运行时公式排版库。
const web = new URL("../", import.meta.url).pathname;
const read = (p: string) => readFileSync(join(web, p), "utf8");
const walk = (dir: string): string[] =>
    readdirSync(dir).flatMap((n) => {
        const p = join(dir, n);
        return statSync(p).isDirectory() ? walk(p) : /\.(tsx?|css|html)$/.test(n) ? [p] : [];
    });

describe("板书主题与固定字体", () => {
    const pkg = JSON.parse(read("package.json")) as { dependencies: Record<string, string>; devDependencies?: Record<string, string> };

    it("字体包精确锁定版本，bun.lock 记录同一版本与 sha512 完整性 hash", () => {
        expect(pkg.dependencies[BOARD_FONT_PACKAGE]).toBe(BOARD_FONT_VERSION);
        const lock = read("bun.lock");
        const line = lock.split("\n").find((l) => l.trim().startsWith(`"${BOARD_FONT_PACKAGE}": ["`));
        expect(line).toBeDefined();
        expect(line).toContain(`"${BOARD_FONT_PACKAGE}@${BOARD_FONT_VERSION}"`);
        expect(line).toMatch(/"sha512-[A-Za-z0-9+/]{86}=="/);
    });

    it("字体包 CSS 声明的字体名与板书主题一致，且只引用包内文件", () => {
        const css = read(`node_modules/${BOARD_FONT_PACKAGE}/index.css`);
        const families = new Set([...css.matchAll(/font-family:\s*'([^']+)'/g)].map((m) => m[1]));
        expect([...families]).toEqual([BOARD_FONT_FACE]);
        const urls = [...css.matchAll(/url\(([^)]+)\)/g)].map((m) => m[1]);
        expect(urls.length).toBeGreaterThan(0);
        for (const u of urls) expect(u.startsWith("./files/")).toBe(true);
        const meta = JSON.parse(read(`node_modules/${BOARD_FONT_PACKAGE}/package.json`)) as { version: string; license: string };
        expect(meta.version).toBe(BOARD_FONT_VERSION);
        expect(meta.license).toBe("OFL-1.1");
    });

    it("入口引入字体包，板书、课堂演示与图形标签使用同一字体栈", () => {
        expect(read("src/main.tsx")).toContain(`import "${BOARD_FONT_PACKAGE}";`);
        expect(BOARD_FONT_FAMILY.startsWith(`"${BOARD_FONT_FACE}",`)).toBe(true);
        expect(BOARD_FONT_FAMILY.endsWith("sans-serif")).toBe(true);
        expect(BOARD_THEME_VERSION).toBe("board-theme-v1");
        for (const f of ["step-viewer.tsx", "classroom.tsx"]) expect(read(`src/components/edu/${f}`)).toContain("style={boardFontStyle}");
        expect(read("src/components/edu/interactive-figure.tsx")).toContain("fontFamily={BOARD_FONT_FAMILY}");
    });

    it("不引入运行时公式排版库，不从外部字体服务加载", () => {
        const deps = Object.keys({ ...pkg.dependencies, ...(pkg.devDependencies ?? {}) });
        expect(deps.filter((d) => /katex|mathjax/i.test(d))).toEqual([]);
        const remote = /fonts\.googleapis\.com|fonts\.gstatic\.com|fonts\.loli\.net|cdn\.jsdelivr\.net\/npm\/katex|use\.typekit\.net/;
        const hits = [join(web, "index.html"), ...walk(join(web, "src"))].filter((p) => remote.test(readFileSync(p, "utf8")));
        expect(hits).toEqual([]);
    });
});
