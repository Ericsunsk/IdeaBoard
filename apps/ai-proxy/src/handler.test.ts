import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { handleDryRun, handleGenerate } from "./handler.js";
import type { Env } from "./contracts.js";

const env: Env = {
  CF_ACCOUNT_ID: "acct123",
  CF_GATEWAY_ID: "gw1",
  CF_AIG_TOKEN: "cf-gateway-token-xyz",
  INTERNAL_SECRET: "internal-abc",
};

function req(
  path: string,
  body: unknown,
  auth = "Bearer internal-abc",
): Request {
  return new Request(`https://edge.test${path}`, {
    method: "POST",
    headers: { Authorization: auth, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

// 成功路径 mock：上游返回 image/png 二进制 + receipt 头；PUT 成功。
// airun（api.cloudflare.com）与 gateway 双 host 均接受。
function mockFetchSuccess(expectedUpload: string) {
  return async (
    url: string | URL | Request,
    init?: RequestInit,
  ): Promise<Response> => {
    const u = String(url instanceof Request ? url.url : url);
    if (
      u.startsWith("https://gateway.ai.cloudflare.com") ||
      u.startsWith("https://api.cloudflare.com/client/v4/accounts/")
    ) {
      const bytes = new Uint8Array([137, 80, 78, 71, 1, 2, 3])
        .buffer as ArrayBuffer;
      return new Response(bytes, {
        status: 200,
        headers: {
          "content-type": "image/png",
          "cf-aig-log-id": "log1",
          "cf-aig-cache-status": "MISS",
        },
      });
    }
    if (u === expectedUpload) {
      assert.equal(init?.method, "PUT");
      // exact key：mock 侧校验 URL 原样（含签名 query）。
      assert.ok(u.includes("sig="));
      return new Response("ok", { status: 200 });
    }
    return new Response("not found", { status: 404 });
  };
}

describe("auth", () => {
  test("INTERNAL_SECRET 缺失 -> 401 fail-closed", async () => {
    const noSecret = { ...env, INTERNAL_SECRET: "" };
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "t1",
        provider: "workers-ai",
        model: "m",
        prompt: "p",
        upload_url: "https://s3/x",
      }),
      noSecret,
    );
    assert.equal(res.status, 401);
  });

  test("Bearer 不匹配 -> 401", async () => {
    const res = await handleGenerate(
      req(
        "/v1/generate",
        {
          task_id: "t1",
          provider: "workers-ai",
          model: "m",
          prompt: "p",
          upload_url: "https://s3/x",
        },
        "Bearer wrong",
      ),
      env,
    );
    assert.equal(res.status, 401);
  });
});

describe("generate validation", () => {
  test("缺少 upload_url -> 400 BAD_REQUEST", async () => {
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "t1",
        provider: "workers-ai",
        model: "m",
        prompt: "p",
      }),
      env,
    );
    assert.equal(res.status, 400);
    const body = (await res.json()) as { error: { code: string } };
    assert.equal(body.error.code, "BAD_REQUEST");
  });

  test("未知 provider -> 422 UNSUPPORTED_MODEL fail-closed", async () => {
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "t1",
        provider: "other",
        model: "m",
        prompt: "p",
        upload_url: "https://s3/x",
      }),
      env,
    );
    assert.equal(res.status, 422);
    const body = (await res.json()) as { error: { code: string } };
    assert.equal(body.error.code, "UNSUPPORTED_MODEL");
  });
});

describe("generate success (recorded, no external)", () => {
  test("workers-ai migrated via airun -> PUT exact + receipt", async () => {
    const upload = "https://s3.example/bucket/key.png?sig=abc&expires=1";
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "t9",
        provider: "workers-ai",
        model: "@cf/m",
        prompt: "a cat",
        upload_url: upload,
        public_url: "https://cdn.example/key.png",
        seed: 7,
      }),
      env,
      mockFetchSuccess(upload) as typeof fetch,
    );
    assert.equal(res.status, 200);
    const body = (await res.json()) as {
      status: string;
      image_url: string;
      content_type: string;
      seed: number;
      receipt: {
        upstream_status: number;
        log_id: string;
        cache_status: string;
      };
    };
    assert.equal(body.status, "completed");
    assert.equal(body.image_url, "https://cdn.example/key.png");
    assert.equal(body.content_type, "image/png");
    assert.equal(body.seed, 7);
    assert.equal(body.receipt.upstream_status, 200);
    assert.equal(body.receipt.log_id, "log1");
    assert.equal(body.receipt.cache_status, "MISS");
    // 密钥永不进响应/日志载荷：响应体无 token。
    assert.ok(!JSON.stringify(body).includes("cf-gateway-token-xyz"));
  });

  test("queue 202 -> 422 UNSUPPORTED_MODEL fail-closed", async () => {
    const mockQueue = async (
      url: string | URL | Request,
    ): Promise<Response> => {
      const u = String(url instanceof Request ? url.url : url);
      if (
        u.startsWith("https://gateway.ai.cloudflare.com") ||
        u.startsWith("https://api.cloudflare.com/client/v4/accounts/")
      ) {
        return new Response(
          JSON.stringify({ request_id: "q", status: "IN_QUEUE" }),
          {
            status: 202,
            headers: { "content-type": "application/json" },
          },
        );
      }
      return new Response("no put expected", { status: 500 });
    };
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "tq",
        provider: "fal",
        model: "fal-ai/m",
        prompt: "p",
        upload_url: "https://s3/x",
      }),
      env,
      mockQueue as typeof fetch,
    );
    assert.equal(res.status, 422);
    const body = (await res.json()) as { error: { code: string } };
    assert.equal(body.error.code, "UNSUPPORTED_MODEL");
  });

  test("输入非 image/* -> 400 INPUT_FETCH_FAILED fail-closed", async () => {
    const mockInput = async (
      url: string | URL | Request,
    ): Promise<Response> => {
      const u = String(url instanceof Request ? url.url : url);
      if (u === "https://cdn.example/evil.json") {
        return new Response("{}", {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response("unexpected", { status: 500 });
    };
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "ti",
        provider: "workers-ai",
        model: "m",
        prompt: "p",
        upload_url: "https://s3/x",
        images: ["https://cdn.example/evil.json"],
      }),
      env,
      mockInput as typeof fetch,
    );
    assert.equal(res.status, 400);
    const body = (await res.json()) as { error: { code: string } };
    assert.equal(body.error.code, "INPUT_FETCH_FAILED");
  });
});

describe("dry-run 自测", () => {
  test("upload_url 可选 + secret 全打码 + path_source", async () => {
    const res = await handleDryRun(
      req("/v1/dry-run", {
        task_id: "d1",
        provider: "fal",
        model: "fal-ai/fast-sdxl",
        prompt: "cat",
        provider_options: { api_key: "should-mask", guidance_scale: 3.5 },
      }),
      env,
    );
    assert.equal(res.status, 200);
    const body = (await res.json()) as {
      provider: string;
      model: string;
      target: { url: string; method: string; body: unknown };
      path_source: string;
    };
    assert.equal(body.provider, "fal");
    assert.ok(body.target.url.includes("/fal/fal-ai/fast-sdxl"));
    assert.equal(body.path_source, "fal:text-to-image");
    const s = JSON.stringify(body);
    assert.ok(!s.includes("cf-gateway-token-xyz"));
    assert.ok(!s.includes("should-mask"));
    assert.ok(s.includes("***"));
  });

  test("dry-run 不需要 upload_url", async () => {
    const res = await handleDryRun(
      req("/v1/dry-run", {
        task_id: "d2",
        provider: "openai",
        model: "gpt-image-1",
        prompt: "p",
      }),
      env,
    );
    assert.equal(res.status, 200);
  });

  test("workers-ai dry-run 标注迁移 + 走 /ai/run", async () => {
    const res = await handleDryRun(
      req("/v1/dry-run", {
        task_id: "d3",
        provider: "workers-ai",
        model: "@cf/m",
        prompt: "p",
      }),
      env,
    );
    assert.equal(res.status, 200);
    const body = (await res.json()) as {
      target: { url: string; body: unknown };
      path_source: string;
    };
    assert.ok(
      body.target.url.endsWith("/accounts/acct123/ai/run"),
      body.target.url,
    );
    assert.ok(body.path_source.startsWith("airun:migrated:workers-ai"));
    assert.ok(JSON.stringify(body.target.body).includes("migration_note"));
  });

  test("dry-run 未知 provider -> 400", async () => {
    const res = await handleDryRun(
      req("/v1/dry-run", {
        task_id: "d4",
        provider: "other",
        model: "m",
        prompt: "p",
      }),
      env,
    );
    assert.equal(res.status, 400);
  });
});
