// Controlled browser fixture for the real Studio UI. No model execution routes.
import { createServer } from "node:http";
import { readFileSync } from "node:fs";

const catalogPath = process.env.STUDIO_PRESENTATION_HOST_FIXTURE;
if (!catalogPath) throw new Error("STUDIO_PRESENTATION_HOST_FIXTURE required");
const features = { welcomeEnabled: true, imageWorkbenchEnabled: true, videoWorkbenchEnabled: true, creditsEnabled: true, taskCenterEnabled: true, customChannelsEnabled: false, frontendModelsEnabled: false, pluginCenterEnabled: false, shortDramaEnabled: false, ecommerceEnabled: false };
const user = { id: "model-presentation-fixture", username: "presentation-fixture", displayName: "模型展示验收夹具", role: "user", status: "active", createdAt: "2026-11-13T00:00:00Z", updatedAt: "2026-11-13T00:00:00Z" };
const payloads = {
    "POST /api/auth/session": { user, provider: "platform", features },
    "GET /api/features": { features },
    "GET /api/user-data/snapshot": { assets: [], projects: [] },
    "GET /api/announcements": { announcements: [], unreadCount: 0 },
    "GET /api/banner-announcements": { banners: [] },
    "GET /api/workbench/tools": { tools: [] },
    "GET /api/workbench/tasks": [],
    "GET /api/tasks": { tasks: [], total: 0 },
    "GET /api/assets": { assets: [], total: 0, hasMore: false },
    "GET /api/canvas-projects": { projects: [], total: 0, hasMore: false },
};
createServer((req, res) => {
    const path = new URL(req.url, "http://127.0.0.1").pathname;
    const key = `${req.method} ${path}`;
    console.log(key);
    res.setHeader("Content-Type", "application/json");
    res.setHeader("Cache-Control", "no-store");
    if (key === "GET /api/model-catalog") {
        let mode = "normal";
        if (process.env.STUDIO_PRESENTATION_MODE_FILE) mode = readFileSync(process.env.STUDIO_PRESENTATION_MODE_FILE, "utf8").trim();
        if (mode === "error") {
            res.statusCode = 503;
            return res.end(JSON.stringify({ code: 503, reason: "model_catalog_unavailable", msg: "受控目录读取失败" }));
        }
        if (mode === "empty") return res.end(JSON.stringify({ code: 0, data: { source: "platform", models: [], channels: [{ id: "oceanway-platform", displayName: "Platform", models: [], platformGroups: [] }] }, msg: "受控空目录" }));
        return res.end(readFileSync(catalogPath));
    }
    if (Object.hasOwn(payloads, key)) return res.end(JSON.stringify({ code: 0, data: payloads[key], msg: "受控展示夹具" }));
    res.statusCode = 503;
    res.end(JSON.stringify({ code: 503, reason: "presentation_fixture_only", msg: "此夹具仅验证模型展示，不提供生成或写入服务" }));
}).listen(19648, "127.0.0.1", () => console.log("Controlled fixture API: 127.0.0.1:19648"));
