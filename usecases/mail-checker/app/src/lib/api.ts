const BASE = import.meta.env.VITE_API_URL || "http://localhost:8080/api/v1";

async function get(path, params: any = {}) {
    const url = new URL(BASE + path);
    Object.entries(params).forEach(([k, v]) => {
        if (v !== undefined && v !== null && v !== "") url.searchParams.set(k, v as any);
    });
    const res = await fetch(url.toString());
    if (!res.ok) throw new Error(`API error ${res.status}`);
    return res.json();
}

export const api = {
    summary: (runId) => get("/summary", { run_id: runId }),
    scoreDistribution: (runId) => get("/score-distribution", { run_id: runId }),
    statusBreakdown: (runId) => get("/status-breakdown", { run_id: runId }),
    failureReasons: (runId) => get("/failure-reasons", { run_id: runId }),
    stagePerformance: (runId) => get("/stage-performance", { run_id: runId }),
    trend: (runId) => get("/trend", { run_id: runId }),
    results: (params) => get("/results", params),
};
