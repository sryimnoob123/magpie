// Run with Node's test runner and Playwright on the module path; see README.md.
// A plugin's provider answers inside magpie, so its plugin:// URLs are no
// address to show. The editor shows the ones the plugin declared for its
// vendor's APIs instead, read-only, each with the API it answers on (#2409):
// nothing here sends a request, so there is no Test and no field to type in.
// A plugin that declared no address shows no Endpoints section at all, as
// before. Both engines, 1100 and 420 wide, every language.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

// a plugin answering on three APIs, as opendesign's does: chat completions,
// Responses for some models, Anthropic's messages for others
const RELAY = {
  chat: "https://relay.example.com/v1",
  responses: "https://relay.example.com/responses",
  anthropic: "https://relay.example.com/messages",
};

const account = (agent, name) => ({ agent, agentName: name, agentIcon: "generic", user: "me@" + agent,
  logins: [{ user: "me@" + agent, active: true, on: true }] });

const providers = [
  { id: "relayco", name: "RelayCo", icon: "generic", chat: "plugin://relayco/v1", responses: "plugin://relayco/v1", anthropic: "plugin://relayco",
    declared: RELAY, models: [{ id: "relay-1", name: "Relay 1", on: true }], agents: [], fallback: [], headers: {}, keyList: [], key: {},
    proxy: "", maxConcurrency: null, account: account("relayco", "RelayCo") },
  // a plugin whose provider declares no address: its editor is as it was
  { id: "silentco", name: "SilentCo", icon: "generic", chat: "plugin://silentco/v1", responses: "", anthropic: "",
    models: [{ id: "silent-1", name: "Silent 1", on: true }], agents: [], fallback: [], headers: {}, keyList: [], key: {},
    proxy: "", maxConcurrency: null, account: account("silentco", "SilentCo") },
];

const plugins = providers.map((p) => ({ id: p.id, pid: p.id, name: p.name, icon: "generic", spec: "opencode-" + p.id + "-auth",
  signedIn: true, models: 1, methods: [{ type: "api", label: "API key" }] }));

function serve(lang) {
  const payload = { providers, presets: [], excluded: [], gateway: { running: true, window: true }, plugins };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/providers") return json(payload);
    if (url.pathname === "/api/groups") return json({ groups: [], pools: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const HINT = "Read-only, as the plugin declared them: magpie sends each request through the plugin, not to these addresses itself.";
const W = {
  en: { field: "Endpoints", hint: HINT, cancel: "Cancel" },
  zh: { field: "端点", hint: "只读，由插件声明：magpie 的每个请求都经由插件发出，不会直接发到这些地址。", cancel: "取消" },
  "zh-TW": { field: "端點", hint: "唯讀，由外掛宣告：magpie 的每個請求都經由外掛送出，不會直接送到這些位址。", cancel: "取消" },
  ja: { field: "エンドポイント", hint: "読み取り専用（プラグインの宣言どおり）：magpie は各リクエストをプラグイン経由で送るため、これらのアドレスへ直接送ることはありません。", cancel: "キャンセル" },
  de: { field: "Endpunkte", hint: "Schreibgeschützt, wie vom Plugin angegeben: magpie sendet jede Anfrage über das Plugin, nicht direkt an diese Adressen.", cancel: "Abbrechen" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a plugin's own endpoints, shown", async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const width of [1100, 420]) for (const lang of Object.keys(W)) {
      await t.test(`${width} ${lang}`, async () => {
        const w = W[lang];
        const ctx = await browser.newContext({ viewport: { width, height: 800 }, reducedMotion: "reduce" });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang));
        await page.goto("http://magpie.test/?view=providers");

        await page.locator(".row.provider", { hasText: "RelayCo" }).click();
        const ed = page.locator("#modal .editor");
        await ed.waitFor();
        await page.waitForTimeout(300); // the editor drawn again as what it asked for comes in
        assert.equal(await ed.locator("label", { hasText: w.field }).count(), 1, "the Endpoints section");
        const box = ed.locator(".eps.declared");
        await box.waitFor();
        // each API the plugin declared, with the address it gave for it
        const rows = await box.locator(".ep").evaluateAll((es) => es.map((e) => [e.querySelector(".pl").textContent, e.querySelector("code").textContent]));
        assert.deepEqual(rows, [["OpenAI", RELAY.chat], ["Responses", RELAY.responses], ["Anthropic", RELAY.anthropic]]);
        // the routing base is nowhere to be read as an address
        assert.equal(await ed.locator("code", { hasText: "plugin://" }).count(), 0);
        // read-only: nothing to test, nothing to type in, and it says so
        assert.equal(await box.locator("button").count(), 0, "a Test was offered");
        assert.equal(await box.locator("input, textarea, select").count(), 0, "a field was offered");
        assert.equal(await ed.locator(".hint", { hasText: w.hint }).count(), 1, "the read-only hint");
        // nothing runs past the editor, however long the address
        const spill = await page.evaluate(() => {
          const e = document.querySelector("#modal .editor"), r = e.getBoundingClientRect();
          return { page: document.documentElement.scrollWidth - document.documentElement.clientWidth,
            rows: [...e.querySelectorAll(".eps.declared, .eps.declared .ep, .eps.declared code")].map((x) => x.getBoundingClientRect().right - r.right) };
        });
        assert.ok(spill.page <= 0, "the page scrolls sideways by " + spill.page);
        for (const s of spill.rows) assert.ok(s <= 0, "an endpoint runs past the editor by " + s);
        if (process.env.ARTIFACT_DIR) await ed.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `plugin-endpoints-${engine}-${width}-${lang}.png`) });

        // a plugin that declared no address: no Endpoints section, as before
        await ed.locator(`button:text-is("${w.cancel}")`).click();
        await ed.waitFor({ state: "detached" });
        await page.locator(".row.provider", { hasText: "SilentCo" }).click();
        await ed.waitFor();
        await page.waitForTimeout(300);
        assert.equal(await ed.locator("label", { hasText: w.field }).count(), 0, "a provider whose plugin declared nothing grew an Endpoints section");
        assert.equal(await ed.locator(".eps").count(), 0);
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
}
