import { describe, test } from "node:test";
import assert from "node:assert/strict";
import worker from "./index.js";
import { handleListProviders } from "./handler.js";
import { listProviders, prepareRoute } from "./providers/index.js";
import type { Env } from "./contracts.js";

const env: Env = {
  CF_ACCOUNT_ID: "acct123",
  CF_GATEWAY_ID: "gw1",
  CF_AIG_TOKEN: "cf-gateway-token-xyz",
  WORKER_INTERNAL_SECRET: "internal-abc",
};

function getReq(auth = "Bearer internal-abc"): Request {
  return new Request("https://edge.test/v1/providers", {
    method: "GET",
    headers: { Authorization: auth },
  });
}

describe("GET /v1/providers", () => {
  test("派生自 provider declarations：至少含 fal 和 workers-ai", () => {
    const providers = listProviders();
    // 兼容 slug 和去重保持稳定。
    assert.ok(providers.includes("fal"));
    assert.ok(providers.includes("workers-ai"));
    assert.ok(!providers.includes("migrated"));
    // openai 跨 airun/gateway 两种 native adapter，去重后只出现一次。
    assert.equal(providers.filter((p) => p === "openai").length, 1);
    // 稳定输出：已排序。
    assert.deepEqual(providers, [...providers].sort());
  });

  test("every advertised provider prepares a real route with its matching decoder", async () => {
    const png = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
    const base64 = Buffer.from(png).toString("base64");
    const cases = [
      {
        provider: "custom-gwllm",
        model: "gpt-image-1",
        suffix: "/custom-gwllm/v1/images/generations",
        payload: { data: [{ b64_json: base64 }] },
      },
      {
        provider: "google",
        model: "nano-banana",
        suffix: "/ai/run",
        payload: { data: [{ b64_json: base64 }] },
      },
      {
        provider: "krea",
        model: "krea-3",
        suffix: "/ai/run",
        payload: { data: [{ b64_json: base64 }] },
      },
      {
        provider: "openai",
        model: "dall-e-3",
        suffix: "/openai/images/generations",
        payload: { data: [{ b64_json: base64 }] },
      },
      {
        provider: "fal",
        model: "fal-ai/flux",
        suffix: "/fal/fal-ai/flux",
        payload: { images: [{ url: "https://output.test/image.png" }] },
      },
      {
        provider: "google-vertex-ai",
        model: "gemini-2.5-flash-image",
        suffix: ":generateContent",
        payload: {
          candidates: [
            {
              content: {
                parts: [
                  { inlineData: { mimeType: "image/png", data: base64 } },
                ],
              },
            },
          ],
        },
      },
      {
        provider: "workers-ai",
        model: "@cf/m",
        suffix: "/ai/run",
        payload: { data: [{ b64_json: base64 }] },
      },
    ];
    assert.deepEqual(
      listProviders(),
      cases.map((entry) => entry.provider).sort(),
    );
    for (const entry of cases) {
      const route = prepareRoute(
        {
          task_id: "capability",
          prompt: "bicycle",
          provider: entry.provider,
          model: entry.model,
          ...(entry.provider === "google-vertex-ai"
            ? { provider_options: { vertex_project: "fixture-project" } }
            : {}),
        },
        env,
        [],
      );
      assert.ok(route.target.url.endsWith(entry.suffix), route.target.url);
      const body = JSON.parse(route.target.body as string);
      if (entry.provider === "google-vertex-ai") {
        assert.equal(body.contents[0].parts[0].text, "bicycle");
      } else if (entry.provider === "fal") {
        assert.equal(body.prompt, "bicycle");
      } else {
        assert.equal(
          body.model,
          entry.provider === "google" || entry.provider === "krea"
            ? `${entry.provider}/${entry.model}`
            : entry.model,
        );
      }
      const decoded = await route.extractImage(
        new Response(JSON.stringify(entry.payload)),
        async () =>
          new Response(png, { headers: { "content-type": "image/png" } }),
      );
      assert.deepEqual(new Uint8Array(decoded.bytes), png);
    }
  });

  test("鉴权与 generate 一致：缺 secret/错 bearer 均 401", async () => {
    const noSecret = await handleListProviders(getReq(), {
      ...env,
      WORKER_INTERNAL_SECRET: "",
    });
    assert.equal(noSecret.status, 401);

    const wrong = await handleListProviders(getReq("Bearer wrong"), env);
    assert.equal(wrong.status, 401);
    const wrongBody = (await wrong.json()) as {
      error: { code: string };
    };
    assert.equal(wrongBody.error.code, "UNAUTHORIZED");

    const ok = await handleListProviders(getReq(), env);
    assert.equal(ok.status, 200);
    const body = (await ok.json()) as { providers: string[] };
    assert.ok(Array.isArray(body.providers));
    assert.ok(body.providers.includes("fal"));
  });

  test("worker 路由：GET 需 auth，POST 同路径 404，/health 不变", async () => {
    const ok = await worker.fetch(getReq(), env);
    assert.equal(ok.status, 200);
    const body = (await ok.json()) as { providers: string[] };
    assert.ok(body.providers.includes("fal"));

    const noAuth = await worker.fetch(
      new Request("https://edge.test/v1/providers", { method: "GET" }),
      env,
    );
    assert.equal(noAuth.status, 401);

    const wrongMethod = await worker.fetch(
      new Request("https://edge.test/v1/providers", {
        method: "POST",
        headers: { Authorization: "Bearer internal-abc" },
      }),
      env,
    );
    assert.equal(wrongMethod.status, 404);

    const health = await worker.fetch(
      new Request("https://edge.test/health", { method: "GET" }),
      env,
    );
    assert.equal(health.status, 200);
  });
});
