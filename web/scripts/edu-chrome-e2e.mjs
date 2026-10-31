/**
 * 数学工作台学生端真实 Chrome E2E（AC-24/25/26，FR-S05）。
 *
 * 无新增依赖：只用 Bun/Node 内置 spawn / fetch / WebSocket / fs。
 * 1. 编译并启动 edumath 的 TestServeBrowserE2E（真实隔离数学服务 + 种子数据）。
 * 2. 启动 Vite DEV（/api 代理到该服务）与 headless Chrome，通过 CDP 驱动 /dev/edu-learn/:id。
 * 3. 断言：开放答案前所有 /api/edu 响应与页面文字都不含答案；先预测后探索；
 *    拖动、键盘、表单与重置都可用；文字说明先于图形；开放后逐步讲解默认静音、带字幕、可续看；
 *    图形使用只以汇总计数上报。
 * 4. 弱网与窄屏（DEV-088、DEV-096）：320px 移动视口、触控、限速高延迟网络，并把交互图形数据改坏使其渲染失败；
 *    断言文字先出现、图形降级为文字说明、页面不横向溢出、核心操作可用、作答与完整讲解照常完成，降级次数到达服务端。
 * 5. 教师课堂页 /dev/edu-teach（DEV-112）：批量重评分、迁移任务、撤回发布都先预览影响、填写原因再确认；
 *    预览不改变服务端状态，确认后的状态从测试后端数据库读取核对。
 * 所有等待都有硬超时；任何断言失败 exit 1。只终止本脚本启动的进程，只删除本脚本创建的临时目录。
 */
import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const WEB = process.cwd();
const BACKEND = resolve(WEB, "../backend");
const CHROME = [process.env.CHROME_BIN, "/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium", "/usr/bin/chromium-browser"].filter(Boolean).find((p) => existsSync(p));
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let failures = 0;
const assert = (ok, name, detail = "") => {
    console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? " — " + detail : ""}`);
    if (!ok) failures += 1;
    return Boolean(ok);
};
const freePort = () =>
    new Promise((res, rej) => {
        const s = createServer();
        s.on("error", rej);
        s.listen(0, "127.0.0.1", () => {
            const { port } = s.address();
            s.close(() => res(port));
        });
    });

function collect(child) {
    const out = { log: "" };
    child.stdout.on("data", (d) => (out.log += d.toString()));
    child.stderr.on("data", (d) => (out.log += d.toString()));
    return out;
}

async function stop(child, name) {
    if (!child || child.exitCode !== null || child.signalCode !== null) return;
    child.kill("SIGTERM");
    const deadline = Date.now() + 8000;
    while (Date.now() < deadline && child.exitCode === null && child.signalCode === null) await sleep(200);
    if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL");
    console.log(`      stopped ${name} (pid=${child.pid})`);
}

async function startBackend(tmp, apiPort) {
    const bin = join(tmp, "edumath.test");
    const build = spawnSync("go", ["test", "-c", "-o", bin, "./internal/edumath/"], { cwd: BACKEND, stdio: "inherit" });
    if (build.status !== 0) throw new Error("go test -c failed");
    const child = spawn(bin, ["-test.run", "^TestServeBrowserE2E$", "-test.v", "-test.timeout", "15m"], {
        cwd: join(BACKEND, "internal/edumath"),
        env: { ...process.env, EDU_E2E_ADDR: `127.0.0.1:${apiPort}`, EDU_MATH_REQUIRE_SERVICE: "1" },
        stdio: ["ignore", "pipe", "pipe"],
    });
    const out = collect(child);
    const deadline = Date.now() + 180000;
    while (Date.now() < deadline) {
        const m = out.log.match(/EDU_E2E_READY (\{.*\})/);
        if (m) return { child, seed: JSON.parse(m[1]), out };
        if (child.exitCode !== null) throw new Error(`backend exited early:\n${out.log}`);
        await sleep(300);
    }
    throw new Error(`backend not ready within 180s:\n${out.log}`);
}

async function startVite(port, apiPort) {
    const entry = join(WEB, "node_modules", "vite", "bin", "vite.js");
    const [cmd, args] = process.versions.bun ? ["bunx", ["vite"]] : [process.execPath, [entry]];
    const child = spawn(cmd, [...args, "--host", "127.0.0.1", "--port", String(port), "--strictPort"], {
        cwd: WEB,
        env: { ...process.env, VITE_API_PROXY_TARGET: `http://127.0.0.1:${apiPort}` },
        stdio: ["ignore", "pipe", "pipe"],
    });
    const out = collect(child);
    const deadline = Date.now() + 120000;
    while (Date.now() < deadline) {
        if (child.exitCode !== null) throw new Error(`Vite exited early:\n${out.log}`);
        try {
            if ((await fetch(`http://127.0.0.1:${port}/`)).ok) return child;
        } catch {
            /* not yet */
        }
        await sleep(500);
    }
    throw new Error(`Vite not ready:\n${out.log}`);
}

async function startChrome(cdpPort, profile) {
    const args = [`--headless=new`, `--remote-debugging-port=${cdpPort}`, `--user-data-dir=${profile}`, "--no-first-run", "--no-default-browser-check", "--window-size=1280,1600"];
    if (process.platform === "linux" || process.env.CI) args.push("--no-sandbox", "--disable-dev-shm-usage");
    args.push("about:blank");
    const child = spawn(CHROME, args, { stdio: ["ignore", "pipe", "pipe"] });
    const out = collect(child);
    const deadline = Date.now() + 60000;
    while (Date.now() < deadline) {
        if (child.exitCode !== null) throw new Error(`Chrome exited early:\n${out.log}`);
        try {
            if ((await fetch(`http://127.0.0.1:${cdpPort}/json/version`)).ok) return child;
        } catch {
            /* not yet */
        }
        await sleep(400);
    }
    throw new Error("Chrome CDP not ready");
}

async function connect(cdpPort) {
    const target = (await (await fetch(`http://127.0.0.1:${cdpPort}/json/list`)).json()).find((t) => t.type === "page");
    const ws = new WebSocket(target.webSocketDebuggerUrl);
    await new Promise((res, rej) => {
        ws.addEventListener("open", res, { once: true });
        ws.addEventListener("error", rej, { once: true });
    });
    const pending = new Map();
    let id = 0;
    const exceptions = [];
    const statuses = [];
    const urls = new Map();
    const bodies = [];
    const listeners = [];
    const send = (method, params = {}) =>
        new Promise((res, rej) => {
            const n = ++id;
            pending.set(n, { res, rej });
            ws.send(JSON.stringify({ id: n, method, params }));
            setTimeout(() => pending.has(n) && (pending.delete(n), rej(new Error(`CDP timeout ${method}`))), 30000);
        });
    ws.addEventListener("message", (ev) => {
        const m = JSON.parse(ev.data);
        if (m.id && pending.has(m.id)) {
            const p = pending.get(m.id);
            pending.delete(m.id);
            return m.error ? p.rej(new Error(JSON.stringify(m.error))) : p.res(m.result);
        }
        const p = m.params;
        for (const [method, fn] of listeners) if (m.method === method) fn(p);
        if (m.method === "Runtime.exceptionThrown") exceptions.push(p?.exceptionDetails?.exception?.description || p?.exceptionDetails?.text);
        if (m.method === "Network.responseReceived" && p.response.url.includes("/api/edu")) {
            urls.set(p.requestId, p.response.url);
            statuses.push({ status: p.response.status, url: p.response.url });
        }
        if (m.method === "Network.loadingFinished" && urls.has(p.requestId)) {
            const url = urls.get(p.requestId);
            send("Network.getResponseBody", { requestId: p.requestId })
                .then((b) => bodies.push({ url, body: b.base64Encoded ? Buffer.from(b.body, "base64").toString("utf8") : b.body }))
                .catch(() => {});
        }
    });
    await send("Runtime.enable");
    await send("Page.enable");
    await send("Network.enable");
    const evaluate = async (expression) => {
        const r = await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
        if (r.exceptionDetails) throw new Error("evaluate: " + (r.exceptionDetails.exception?.description || r.exceptionDetails.text));
        return r.result.value;
    };
    const poll = async (expression, label, timeout = 20000) => {
        const deadline = Date.now() + timeout;
        let last;
        while (Date.now() < deadline) {
            last = await evaluate(expression);
            if (last) return last;
            await sleep(200);
        }
        console.log(`      (poll timed out: ${label}, last=${JSON.stringify(last)})`);
        return false;
    };
    const center = (sel) =>
        evaluate(`(() => { const el = ${sel}; if (!el) return null; el.scrollIntoView({block: "center"}); const r = el.getBoundingClientRect(); return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) }; })()`);
    const mouse = (type, x, y, buttons = 0) => send("Input.dispatchMouseEvent", { type, x, y, button: "left", buttons, clickCount: 1, pointerType: "mouse" });
    const click = async (sel, label) => {
        const ok = await poll(`(() => { const el = ${sel}; return !!el && !el.disabled; })()`, label, 15000);
        if (!ok) return false;
        await sleep(150);
        const c = await center(sel);
        await mouse("mouseMoved", c.x, c.y);
        await mouse("mousePressed", c.x, c.y, 1);
        await mouse("mouseReleased", c.x, c.y);
        return true;
    };
    const byTestId = (id) => `document.querySelector('[data-testid="${id}"]')`;
    const byText = (text) => `[...document.querySelectorAll("button")].find((b) => (b.textContent || "").trim() === ${JSON.stringify(text)})`;
    const key = async (k, code, vk) => {
        await send("Input.dispatchKeyEvent", { type: "rawKeyDown", key: k, code, windowsVirtualKeyCode: vk });
        await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, windowsVirtualKeyCode: vk });
    };
    const on = (method, fn) => {
        listeners.push([method, fn]);
        return () =>
            listeners.splice(
                listeners.findIndex((l) => l[1] === fn),
                1,
            );
    };
    const tap = async (sel, label) => {
        const ok = await poll(`(() => { const el = ${sel}; return !!el && !el.disabled; })()`, label, 30000);
        if (!ok) return false;
        await sleep(150);
        const c = await center(sel);
        await send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: c.x, y: c.y }] });
        await send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
        return true;
    };
    return { send, evaluate, poll, click, tap, center, mouse, on, byTestId, byText, key, exceptions, statuses, bodies, close: () => ws.close() };
}

async function run(cdp, base, api, seed) {
    const { assignmentId: as, tenant, answerMarkers: markers } = seed;
    const leaks = (s) => markers.filter((m) => s.includes(m));
    await cdp.send("Page.navigate", { url: `${base}/dev/edu-learn/${as}?tenant=${encodeURIComponent(tenant)}` });
    assert(await cdp.poll(`!!${cdp.byTestId("prediction-panel")} && !!${cdp.byTestId("figure-svg")}`, "page mounted", 60000), "E1 学生作业页挂载，预测面板与交互图形出现");

    const order = await cdp.evaluate(`(() => { const d = ${cdp.byTestId("figure-description")}; const s = ${cdp.byTestId("figure-svg")}; return !!d && !!(d.compareDocumentPosition(s) & Node.DOCUMENT_POSITION_FOLLOWING); })()`);
    assert(order, "E2 文字说明先于图形呈现");
    assert((await cdp.evaluate(`${cdp.byTestId("figure-svg")}.getAttribute("tabindex")`)) === "-1", "E3 预测前图形不可操作");

    assert(await cdp.click(cdp.byTestId("prediction-option-3"), "prediction"), "E4 选择预测（说不准）");
    assert(await cdp.poll(`!!${cdp.byTestId("prediction-recorded")} && ${cdp.byTestId("figure-svg")}.getAttribute("tabindex") === "0"`, "prediction recorded"), "E5 预测已记录，图形可操作");

    const yaw = () => cdp.evaluate(`Number(${cdp.byTestId("figure-svg")}.dataset.yaw)`);
    const pitch = () => cdp.evaluate(`Number(${cdp.byTestId("figure-svg")}.dataset.pitch)`);
    const y0 = await yaw();
    await cdp.evaluate(`${cdp.byTestId("figure-svg")}.focus()`);
    await cdp.key("ArrowRight", "ArrowRight", 39);
    await cdp.key("ArrowUp", "ArrowUp", 38);
    assert((await cdp.poll(`Number(${cdp.byTestId("figure-svg")}.dataset.yaw) === ${y0 + 10}`, "key yaw")) && (await pitch()) === 32, "E6 键盘方向键旋转", `yaw ${y0} → ${await yaw()}, pitch ${await pitch()}`);

    const c = await cdp.center(cdp.byTestId("figure-svg"));
    await cdp.mouse("mouseMoved", c.x, c.y);
    await cdp.mouse("mousePressed", c.x, c.y, 1);
    for (let i = 1; i <= 6; i++) await cdp.mouse("mouseMoved", c.x + i * 10, c.y, 1);
    await cdp.mouse("mouseReleased", c.x + 60, c.y);
    const afterDrag = await yaw();
    assert(afterDrag > y0 + 20, "E7 鼠标拖动旋转", `yaw=${afterDrag}`);

    assert(await cdp.click(cdp.byTestId("figure-reset"), "reset"), "E8a 点击重置");
    assert(await cdp.poll(`${cdp.byTestId("figure-svg")}.dataset.yaw === "${y0}" && ${cdp.byTestId("figure-svg")}.dataset.pitch === "22"`, "reset"), "E8 重置回到初始视角");

    // Form input (equivalent non-pointer operation).
    await cdp.evaluate(`${cdp.byTestId("figure-yaw")}.focus(); ${cdp.byTestId("figure-yaw")}.select()`);
    await cdp.send("Input.insertText", { text: "40" });
    assert(await cdp.poll(`${cdp.byTestId("figure-svg")}.dataset.yaw === "40"`, "form yaw"), "E9 表单输入角度");

    // Submit an attempt so hints 1..4 open; level 5 must still be refused by the server.
    await cdp.evaluate(`document.querySelector('input[maxlength="200"]').focus()`);
    await cdp.send("Input.insertText", { text: "1" });
    assert(await cdp.click(cdp.byText("提交"), "submit"), "E10 提交一次作答");
    assert(await cdp.poll(`document.body.innerText.includes("已记录：")`, "attempt recorded"), "E11 作答已记录");
    for (let lv = 1; lv <= 4; lv++) {
        await cdp.click(cdp.byText(`获取第 ${lv} 层提示`), `hint ${lv}`);
        await cdp.poll(`document.body.innerText.includes("第 ${lv} 层")`, `hint ${lv} shown`);
    }
    assert(await cdp.click(cdp.byText("查看完整讲解"), "level5"), "E12a 请求完整讲解");
    assert(await cdp.poll(`document.body.innerText.includes("老师还没有开放答案")`, "refused"), "E12 未开放时服务端拒绝完整讲解");
    await sleep(800);
    const pre = cdp.bodies.map((b) => b.body).join("\n") + "\n" + (await cdp.evaluate("document.body.innerText"));
    assert(cdp.bodies.length >= 5 && leaks(pre).length === 0, "E13 开放前所有 /api/edu 响应与页面文字都不含答案（AC-24）", `responses=${cdp.bodies.length} leaks=${JSON.stringify(leaks(pre))}`);

    const rel = await fetch(`${api}/__e2e/release`, { method: "POST" });
    assert(rel.status === 200, "E14 教师开放答案", `status=${rel.status}`);
    assert(await cdp.click(cdp.byText("查看完整讲解"), "level5 again"), "E15a 再次请求完整讲解");
    assert(await cdp.poll(`!!${cdp.byTestId("step-viewer")}`, "step viewer"), "E15 逐步讲解出现");
    const sv = await cdp.evaluate(`(() => ({
        silent: !!${cdp.byTestId("silent-mode")},
        caption: (${cdp.byTestId("step-caption")}?.textContent || "").length,
        audioDisabled: [...document.querySelectorAll("button")].some((b) => b.textContent.includes("播放语音") && b.disabled),
        step: ${cdp.byTestId("step-viewer")}.dataset.step,
    }))()`);
    assert(sv.silent && sv.caption > 0 && sv.audioDisabled && sv.step === "0", "E16 默认静音模式、字幕可见、语音控件不可用（AC-26）", JSON.stringify(sv));
    await sleep(500);
    const post = cdp.bodies
        .filter((b) => b.url.includes("/hints"))
        .map((b) => b.body)
        .join("\n");
    assert(markers.length > 0 && leaks(post).length > 0, "E16b 对照：开放后的完整讲解响应确实含答案，说明泄漏检查有效", `markers=${markers.length}`);
    await cdp.click(cdp.byTestId("step-next"), "next");
    assert(await cdp.poll(`${cdp.byTestId("step-viewer")}.dataset.step === "1"`, "next step"), "E17 下一步");
    assert((await cdp.evaluate(`localStorage.getItem("edu.step.v1:anon:${as}")`)) === "1", "E18 续看只保存步骤序号");

    await cdp.evaluate(`Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true }); document.dispatchEvent(new Event("visibilitychange")); true`);
    let summary = null;
    for (let i = 0; i < 40 && !summary; i++) {
        const r = await (await fetch(`${api}/__e2e/summary`)).json();
        if (r?.data?.figure?.studentsUsed === 1) summary = r.data;
        else await sleep(250);
    }
    assert(summary && summary.figure.studentsKeyboardOrForm === 1 && summary.predictions.foot?.[0]?.count === 1, "E19 图形使用与预测以汇总形式到达教师端", JSON.stringify(summary));

    // Variant set self-test (FR-S08): no result or key before the last item, results after.
    const { setAssignmentId: sas, setItems, setMarkers } = seed;
    const leaksSet = (t) => setMarkers.filter((m) => t.includes(m));
    const before = cdp.bodies.length;
    await cdp.send("Page.navigate", { url: `${base}/dev/edu-learn/${sas}?tenant=${encodeURIComponent(tenant)}` });
    assert(await cdp.poll(`document.body.innerText.includes("开始自测")`, "set page"), "E22a 题组作业页挂载");
    assert(await cdp.click(cdp.byText("开始自测"), "start set"), "E22b 开始题组自测");
    assert(await cdp.poll(`!!${cdp.byTestId("set-self-test")} && document.querySelectorAll('[data-testid="set-self-test"] li').length === ${setItems}`, "set questions"), "E22 题组题目全部出现", `items=${setItems}`);
    for (let i = 0; i < setItems; i++) {
        await cdp.evaluate(`document.querySelector('[data-testid="set-self-test"] input').focus()`);
        await cdp.send("Input.insertText", { text: "1" });
        await cdp.click(cdp.byText("提交本题"), `set item ${i}`);
        if (i < setItems - 1) {
            assert(await cdp.poll(`document.body.innerText.includes("还剩 ${setItems - i - 1} 题")`, "set progress"), `E23.${i} 第 ${i + 1} 题后只显示已记录`);
            await sleep(400);
            const mid =
                cdp.bodies
                    .slice(before)
                    .map((b) => b.body)
                    .join("\n") +
                "\n" +
                (await cdp.evaluate("document.body.innerText"));
            const graded = /"result":"(correct|incorrect|correct_approx|partial)"/.test(
                cdp.bodies
                    .slice(before)
                    .filter((b) => b.url.includes("/attempts") || b.url.includes("/learning") || b.url.includes("/progress"))
                    .map((b) => b.body)
                    .join("\n"),
            );
            assert(leaksSet(mid).length === 0 && !graded, `E24.${i} 未全部提交前响应与页面不含题组答案或对错`, `leaks=${JSON.stringify(leaksSet(mid))} graded=${graded}`);
        }
    }
    assert(await cdp.poll(`document.body.innerText.includes("已全部提交") && document.body.innerText.includes("参考答案")`, "set results"), "E25 全部提交后显示结果与参考答案");
    const after = cdp.bodies
        .slice(before)
        .map((b) => b.body)
        .join("\n");
    assert(setMarkers.length > 0 && leaksSet(after).length > 0, "E25b 对照：提交后响应确实含题组答案，说明检查有效", `markers=${setMarkers.length}`);

    await runWeak(cdp, base, api, seed);
    await runTeacher(cdp, base, api, seed.teacher);

    const bad = cdp.statuses.filter((s) => s.status >= 400 && !(s.status === 409 && s.url.includes("/hints")));
    assert(bad.length === 0, "E20 /api/edu 无意外错误响应", JSON.stringify(bad));
    assert(cdp.exceptions.length === 0, "E21 页面无未捕获异常", JSON.stringify(cdp.exceptions));
}

/** DEV-088 and DEV-096: a 320px touch phone on a slow, high latency network whose figure fails to render. */
async function runWeak(cdp, base, api, seed) {
    const { weakAssignmentId: wid, tenant } = seed;
    const weakState = async () => (await fetch(`${api}/__e2e/weak-state`)).json();
    await cdp.send("Emulation.setDeviceMetricsOverride", { width: 320, height: 720, deviceScaleFactor: 2, mobile: true });
    await cdp.send("Emulation.setTouchEmulationEnabled", { enabled: true, maxTouchPoints: 5 });
    await cdp.send("Network.emulateNetworkConditions", { offline: false, latency: 400, downloadThroughput: 50 * 1024, uploadThroughput: 20 * 1024 });
    // Break the figure data in the lesson response so the interactive figure throws while rendering,
    // as a failed or unsupported 3D view would; everything else is passed through untouched.
    let broken = 0;
    const off = cdp.on("Fetch.requestPaused", async (p) => {
        try {
            if (new URL(p.request.url).pathname.endsWith(`/student/assignments/${wid}`) && p.responseStatusCode === 200) {
                const raw = await cdp.send("Fetch.getResponseBody", { requestId: p.requestId });
                const body = JSON.parse(raw.base64Encoded ? Buffer.from(raw.body, "base64").toString("utf8") : raw.body);
                const fig = body?.data?.lesson?.figure;
                if (fig?.explore?.mode === "rotate") {
                    delete fig.explore.reset;
                    broken += 1;
                }
                return await cdp.send("Fetch.fulfillRequest", { requestId: p.requestId, responseCode: 200, responseHeaders: p.responseHeaders, body: Buffer.from(JSON.stringify(body)).toString("base64") });
            }
            await cdp.send("Fetch.continueRequest", { requestId: p.requestId });
        } catch (e) {
            console.log("      (fetch intercept: " + e + ")");
            await cdp.send("Fetch.continueRequest", { requestId: p.requestId }).catch(() => {});
        }
    });
    await cdp.send("Fetch.enable", { patterns: [{ urlPattern: `*/api/edu/student/assignments/${wid}*`, requestStage: "Response" }] });
    try {
        const t0 = Date.now();
        await cdp.send("Page.navigate", { url: `${base}/dev/edu-learn/${wid}?tenant=${encodeURIComponent(tenant)}` });
        assert(await cdp.poll(`!!${cdp.byTestId("content-provenance")}`, "weak page text", 60000), "W1 限速高延迟下题目文字出现", `${Date.now() - t0} ms`);
        assert(
            (await cdp.poll(`!!${cdp.byTestId("figure-fallback")} && !${cdp.byTestId("figure-svg")} && (${cdp.byTestId("figure-fallback")}.textContent || "").trim().length > 0`, "figure fallback", 30000)) && broken === 1,
            "W2 交互图形渲染失败时降级为文字说明，页面其余部分照常",
            `broken=${broken}`,
        );
        const layout = await cdp.evaluate(`(() => {
            const w = window.innerWidth;
            const pick = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { l: Math.round(r.left), r: Math.round(r.right) }; };
            const btn = [...document.querySelectorAll("button")].find((b) => (b.textContent || "").trim() === "提交");
            const items = { input: pick(document.querySelector('input[maxlength="200"]')), submit: pick(btn), prediction: pick(${cdp.byTestId("prediction-panel")}), fallback: pick(${cdp.byTestId("figure-fallback")}) };
            return { w, scroll: document.documentElement.scrollWidth, items, inside: Object.values(items).every((x) => x && x.l >= 0 && x.r <= w + 1) };
        })()`);
        assert(layout.w === 320 && layout.scroll <= 321 && layout.inside, "W3 320px 宽无横向溢出，作答框、提交、预测与图形说明都在可视宽度内", JSON.stringify(layout));
        assert(await cdp.tap(cdp.byTestId("prediction-option-3"), "weak prediction"), "W4a 触控选择预测");
        assert(await cdp.poll(`!!${cdp.byTestId("prediction-recorded")}`, "weak prediction recorded", 30000), "W4 图形降级后预测仍可记录");
        await cdp.evaluate(`document.querySelector('input[maxlength="200"]').focus()`);
        await cdp.send("Input.insertText", { text: "1" });
        assert(await cdp.tap(cdp.byText("提交"), "weak submit"), "W5a 触控提交作答");
        assert(await cdp.poll(`document.body.innerText.includes("已记录：")`, "weak attempt", 30000), "W5b 页面显示已记录");
        assert((await weakState()).attempts === 1, "W5 服务端收到 1 次作答");
        for (let lv = 1; lv <= 4; lv++) {
            await cdp.tap(cdp.byText(`获取第 ${lv} 层提示`), `weak hint ${lv}`);
            await cdp.poll(`document.body.innerText.includes("第 ${lv} 层")`, `weak hint ${lv} shown`, 30000);
        }
        const rel = await fetch(`${api}/__e2e/weak-release`, { method: "POST" });
        assert(rel.status === 200, "W6a 教师开放答案", `status=${rel.status}`);
        assert(await cdp.tap(cdp.byText("查看完整讲解"), "weak level5"), "W6b 触控请求完整讲解");
        assert(await cdp.poll(`!!${cdp.byTestId("step-viewer")}`, "weak step viewer", 30000), "W6 弱网下逐步讲解出现");
        const sv = await cdp.evaluate(`(() => { const v = ${cdp.byTestId("step-viewer")}; const r = v.getBoundingClientRect();
            return { silent: !!${cdp.byTestId("silent-mode")}, caption: (${cdp.byTestId("step-caption")}?.textContent || "").length, l: Math.round(r.left), r: Math.round(r.right), w: window.innerWidth, scroll: document.documentElement.scrollWidth }; })()`);
        assert(sv.silent && sv.caption > 0 && sv.l >= 0 && sv.r <= sv.w + 1 && sv.scroll <= sv.w + 1, "W7 无语音时静音字幕模式完成讲解，播放器在 320px 内", JSON.stringify(sv));
        await cdp.tap(cdp.byTestId("step-next"), "weak next");
        assert(await cdp.poll(`${cdp.byTestId("step-viewer")}.dataset.step === "1"`, "weak next step", 30000), "W8 触控翻到下一步");
        await cdp.evaluate(`Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true }); document.dispatchEvent(new Event("visibilitychange")); true`);
        let fig = null;
        for (let i = 0; i < 60 && !fig; i++) {
            const st = await weakState();
            if (st?.figure?.fallbacks >= 1) fig = st.figure;
            else await sleep(250);
        }
        assert(fig?.fallbacks === 1, "W9 图形降级次数以汇总形式到达教师端", JSON.stringify(fig));
    } finally {
        off();
        await cdp.send("Fetch.disable");
        await cdp.send("Network.emulateNetworkConditions", { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 });
        await cdp.send("Emulation.setTouchEmulationEnabled", { enabled: false });
        await cdp.send("Emulation.clearDeviceMetricsOverride");
    }
}

async function runTeacher(cdp, base, api, t) {
    const state = async () => (await fetch(`${api}/__e2e/teacher-state`)).json();
    const setSelect = (label, value) =>
        cdp.evaluate(`(() => { const el = document.querySelector('select[aria-label="${label}"]'); if (!el || ![...el.options].some((o) => o.value === ${JSON.stringify(value)})) return false;
            Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event("change", { bubbles: true })); return true; })()`);
    const type = async (testId, text) => {
        await cdp.evaluate(`${cdp.byTestId(testId)}.focus()`);
        await cdp.send("Input.insertText", { text });
    };
    const open = (summary) => cdp.evaluate(`(() => { const s = [...document.querySelectorAll("summary")].find((x) => x.textContent.trim() === ${JSON.stringify(summary)}); if (!s) return false; s.parentElement.open = true; return true; })()`);

    await cdp.send("Page.navigate", { url: `${base}/dev/edu-teach` });
    assert(await cdp.poll(`document.querySelector('select[aria-label="班级"]')?.options.length > 1`, "teacher classes", 60000), "T1 教师课堂页挂载并列出班级");
    assert(await setSelect("班级", t.classId), "T2a 选择班级");
    assert(await cdp.poll(`[...document.querySelector('select[aria-label="作业"]').options].some((o) => o.value === ${JSON.stringify(t.assignmentId)})`, "teacher assignments"), "T2b 列出作业");
    assert(await setSelect("作业", t.assignmentId), "T2 选择作业");
    assert(await cdp.poll(`[...document.querySelectorAll("summary")].some((x) => x.textContent.trim() === "批量重评分")`, "action panels"), "T3 迁移、重评分、撤回发布入口出现");

    // Teaching metrics (DEV-117): denominators and the definition version are always shown.
    assert(await open("教学指标（带分母）"), "T3b 打开教学指标");
    assert(
        await cdp.poll(`/在读学生 1 人/.test(${cdp.byTestId("teaching-metrics")}?.textContent || "") && (${cdp.byTestId("teaching-metrics")}?.textContent || "").includes("teaching_metrics/1")`, "teaching metrics"),
        "T3c 教学指标显示分母与口径版本",
        await cdp.evaluate(`${cdp.byTestId("teaching-metrics")}?.textContent || ""`),
    );

    // Batch regrade: the reason is required, the preview lists the change and writes nothing.
    await open("批量重评分");
    assert(await cdp.click(cdp.byTestId("regrade-preview"), "regrade preview empty"), "T4a 无原因预览");
    assert(await cdp.poll(`(${cdp.byTestId("regrade-status")}?.textContent || "").includes("原因")`, "reason required"), "T4 未填原因时在本地提示，不发请求确认");
    await type("regrade-reason", "答案键更正后重新评分");
    assert(await cdp.click(cdp.byTestId("regrade-preview"), "regrade preview"), "T5a 预览重评分");
    assert(await cdp.poll(`/1 次结果会改变/.test(${cdp.byTestId("regrade-impact")}?.textContent || "")`, "regrade impact"), "T5 预览显示 1 次作答会改变", await cdp.evaluate(`${cdp.byTestId("regrade-impact")}?.textContent || ""`));
    let st = await state();
    assert(st.attemptResult === "correct" && !st.audit.includes("assignment.regrade"), "T6 预览不改变作答结果、不写审计", JSON.stringify({ r: st.attemptResult }));
    assert(await cdp.click(cdp.byTestId("regrade-confirm"), "regrade confirm"), "T7a 确认重评分");
    assert(await cdp.poll(`(${cdp.byTestId("regrade-status")}?.textContent || "").includes("已重评分：1 次")`, "regraded"), "T7b 页面显示已重评分");
    st = await state();
    assert(st.attemptResult === "incorrect" && st.audit.includes("assignment.regrade"), "T7 服务端结果已更正并写入审计", JSON.stringify({ r: st.attemptResult }));

    // Migration to the newer publication: preview, then confirm; the old attempts stay put.
    await open("迁移任务到新发布版本");
    assert(await setSelect("迁移到的发布版本", t.fixPublicationId), "T8a 选择新发布版本");
    assert((await cdp.evaluate(`[...document.querySelector('select[aria-label="迁移到的发布版本"]').options].some((o) => o.value === ${JSON.stringify(t.publicationId)})`)) === false, "T8b 当前发布不在迁移目标中");
    await type("migrate-reason", "讲解措辞修正，迁移到新版本");
    assert(await cdp.click(cdp.byTestId("migrate-preview"), "migrate preview"), "T8c 预览迁移");
    assert(await cdp.poll(`/1 名学生/.test(${cdp.byTestId("migrate-impact")}?.textContent || "")`, "migrate impact"), "T8 迁移预览显示受影响学生", await cdp.evaluate(`${cdp.byTestId("migrate-impact")}?.textContent || ""`));
    st = await state();
    assert(st.assignments.length === 1 && st.assignments[0].state === "active", "T9 预览不迁移", JSON.stringify(st.assignments));
    assert(await cdp.click(cdp.byTestId("migrate-confirm"), "migrate confirm"), "T10a 确认迁移");
    assert(await cdp.poll(`document.body.innerText.includes("已迁移到新任务并切换过去")`, "migrated"), "T10b 页面显示已迁移");
    st = await state();
    const moved = st.assignments.find((a) => a.publicationId === t.fixPublicationId);
    assert(st.assignments.find((a) => a.id === t.assignmentId)?.state === "migrated" && moved?.state === "active", "T10 服务端旧任务为已迁移、新任务在新版本上", JSON.stringify(st.assignments));
    assert(await cdp.poll(`document.querySelector('select[aria-label="作业"]').value === ${JSON.stringify(moved?.id || "")}`, "new assignment selected"), "T11 页面自动切到新任务");

    // Withdraw the new publication: the preview names the impact and leaves it published.
    await open("撤回发布（内容有错）");
    await type("withdraw-pub-reason", "第二步的数值算错，需要停止使用");
    assert(await cdp.click(cdp.byTestId("withdraw-pub-preview"), "withdraw preview"), "T12a 预览撤回发布");
    assert(await cdp.poll(`/涉及 1 个任务/.test(${cdp.byTestId("withdraw-pub-impact")}?.textContent || "")`, "withdraw impact"), "T12 撤回预览列出影响", await cdp.evaluate(`${cdp.byTestId("withdraw-pub-impact")}?.textContent || ""`));
    st = await state();
    assert(st.publications[t.fixPublicationId] === "published", "T13 预览不撤回", JSON.stringify(st.publications));
    assert(await cdp.click(cdp.byTestId("withdraw-pub-confirm"), "withdraw confirm"), "T14a 确认撤回发布");
    assert(await cdp.poll(`(${cdp.byTestId("withdraw-pub-status")}?.textContent || "").includes("已撤回发布")`, "withdrawn"), "T14b 页面显示已撤回");
    st = await state();
    assert(st.publications[t.fixPublicationId] === "withdrawn" && st.publications[t.publicationId] === "published" && st.assignments.find((a) => a.id === moved?.id)?.state === "suspended", "T14 服务端只撤回所选发布，相关任务暂停", JSON.stringify(st));
}

async function main() {
    if (!CHROME) throw new Error("No Chrome binary found; set CHROME_BIN");
    const tmp = mkdtempSync(join(tmpdir(), "edu-e2e-"));
    const [apiPort, vitePort, cdpPort] = [await freePort(), await freePort(), await freePort()];
    let backend, vite, chrome, cdp;
    try {
        backend = await startBackend(tmp, apiPort);
        console.log(`      backend ready: ${JSON.stringify(backend.seed.assignmentId)}`);
        vite = await startVite(vitePort, apiPort);
        chrome = await startChrome(cdpPort, join(tmp, "profile"));
        cdp = await connect(cdpPort);
        await run(cdp, `http://127.0.0.1:${vitePort}`, `http://127.0.0.1:${apiPort}`, backend.seed);
    } catch (e) {
        failures += 1;
        console.log("FAIL  harness — " + (e?.stack || e));
        if (backend?.out) console.log(backend.out.log.slice(-4000));
    } finally {
        cdp?.close();
        try {
            await fetch(`http://127.0.0.1:${apiPort}/__e2e/done`);
        } catch {
            /* already down */
        }
        await sleep(500);
        await stop(chrome?.child ?? chrome, "chrome");
        await stop(vite, "vite");
        await stop(backend?.child, "backend");
        rmSync(tmp, { recursive: true, force: true });
    }
    console.log(failures ? `\n${failures} failure(s)` : "\nall edu browser checks passed");
    process.exit(failures ? 1 : 0);
}

main();
