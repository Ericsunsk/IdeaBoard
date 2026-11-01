import { describe, test } from "node:test";
import assert from "node:assert/strict";
import worker from "./index.js";
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
      const bytes = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])
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

test("invalid image roles fail before any fetch in generate and dry-run", async () => {
  for (const mask_index of [-1, 2, 0.5, null]) {
    const body = {
      task_id: "roles",
      provider: "fal",
      model: "m",
      prompt: "p",
      images: ["https://media.test/1", "https://media.test/2"],
      mask_index,
      upload_url: "https://media.test/out",
    };
    const res = await handleGenerate(
      req("/v1/generate", body),
      env,
      async () => {
        throw new Error("must not fetch");
      },
    );
    assert.equal(res.status, 400);
    assert.equal(
      (await handleDryRun(req("/v1/dry-run", body), env)).status,
      400,
    );
  }
});

test("every Gateway transport enforces privacy/cache policy on outgoing requests", async () => {
  for (const [provider, model, images] of [
    ["fal", "flux", undefined],
    ["openai", "dall-e-2", undefined],
    ["openai", "dall-e-2", ["https://media.test/input"]],
    ["workers-ai", "@cf/m", undefined],
  ] as const) {
    let upstream = 0;
    const res = await handleGenerate(
      req("/v1/generate", {
        task_id: "policy",
        provider,
        model,
        prompt: "p",
        images,
        upload_url: "https://media.test/output",
        provider_options: {
          "cf-aig-skip-cache": false,
          "cf-aig-collect-log-payload": true,
        },
      }),
      env,
      async (url, init) => {
        if (String(url) === "https://media.test/input")
          return new Response(new Uint8Array([137, 80, 78, 71]), {
            headers: { "content-type": "image/png" },
          });
        upstream++;
        const headers = new Headers(init?.headers);
        assert.equal(headers.get("cf-aig-skip-cache"), "true");
        assert.equal(headers.get("cf-aig-collect-log-payload"), "false");
        return new Response("rejected", { status: 400 });
      },
    );
    assert.equal(res.status, 400);
    assert.equal(upstream, 1);
  }
});

test("JPEG and WebP upstream results PUT and report their actual MIME", async () => {
  for (const [mime, raw] of [
    ["image/jpeg", [255, 216, 255, 224, 0]],
    ["image/webp", [...Buffer.from("RIFF0000WEBPVP8 ")]],
  ] as const) {
    for (const [provider, model] of [
      ["workers-ai", "@cf/m"],
      ["openai", "gpt-image-1"],
      ["openai", "dall-e-3"],
    ]) {
      let put = 0;
      const res = await handleGenerate(
        req("/v1/generate", {
          task_id: "mime",
          provider,
          model,
          prompt: "p",
          upload_url: "https://media.test/out",
        }),
        env,
        async (url, init) => {
          if (init?.method === "PUT") {
            put++;
            assert.equal(new Headers(init.headers).get("content-type"), mime);
            assert.deepEqual(
              new Uint8Array(init.body as ArrayBuffer),
              new Uint8Array(raw),
            );
            return new Response(null, { status: 200 });
          }
          if (provider === "openai") {
            return Response.json({
              data: [{ b64_json: Buffer.from(raw).toString("base64") }],
            });
          }
          return new Response(new Uint8Array(raw), {
            headers: { "content-type": mime },
          });
        },
      );
      assert.equal(res.status, 200);
      assert.equal(
        ((await res.json()) as { content_type: string }).content_type,
        mime,
      );
      assert.equal(put, 1);
    }
  }
});

test("all channels reject HTTP 202 before parsing or uploading", async () => {
  for (const [provider, model] of [
    ["workers-ai", "@cf/m"],
    ["openai", "dall-e-2"],
    ["fal", "flux"],
    ["google", "nano-banana"],
  ]) {
    for (const binary of [false, true]) {
      let puts = 0;
      const res = await handleGenerate(
        req("/v1/generate", {
          task_id: "async",
          provider,
          model,
          prompt: "p",
          upload_url: "https://media.test/out",
        }),
        env,
        async (_url, init) => {
          if (init?.method === "PUT") {
            puts++;
            return new Response(null);
          }
          return binary
            ? new Response(new Uint8Array([137, 80, 78, 71]), {
                status: 202,
                headers: { "content-type": "image/png" },
              })
            : Response.json(
                { request_id: "queued", status: "IN_QUEUE" },
                { status: 202 },
              );
        },
      );
      assert.equal(res.status, 422);
      const body = (await res.json()) as {
        status: string;
        error: { code: string; upstream_status: number };
      };
      assert.equal(body.status, "failed");
      assert.equal(body.error.code, "UNSUPPORTED_MODEL");
      assert.equal(body.error.upstream_status, 202);
      assert.equal(puts, 0);
    }
  }
});

test("Worker entry rejects null/array/primitive JSON on both endpoints", async () => {
  for (const path of ["/v1/generate", "/v1/dry-run"]) {
    for (const body of [null, [], 1, "text", false]) {
      const res = await worker.fetch(req(path, body), env);
      assert.equal(res.status, 400);
      const result = (await res.json()) as {
        task_id: string;
        status: string;
        error: { code: string };
      };
      assert.equal(result.task_id, "unknown");
      assert.equal(result.status, "failed");
      assert.equal(result.error.code, "BAD_REQUEST");
    }
  }
});

test("Worker entry catches async handler rejection without exposing error secrets", async () => {
  const secret = "secret-error-token-do-not-expose";
  const brokenEnv = {
    ...env,
    get CF_ACCOUNT_ID(): string {
      throw new Error(secret);
    },
  };
  const logs: string[] = [];
  const previousLog = console.log;
  console.log = (...args: unknown[]) => {
    logs.push(args.join(" "));
  };
  try {
    const res = await worker.fetch(
      req("/v1/dry-run", {
        task_id: "failure",
        provider: "fal",
        model: "flux",
        prompt: "p",
      }),
      brokenEnv,
    );
    assert.equal(res.status, 500);
    const text = await res.text();
    assert.equal(JSON.parse(text).error.code, "ADAPTER_INTERNAL_ERROR");
    assert.ok(!text.includes(secret));
    assert.ok(!logs.join(" ").includes(secret));
  } finally {
    console.log = previousLog;
  }
});

test("unexpected async fetch failure returns safe canonical JSON", async () => {
  const previousFetch = globalThis.fetch;
  const secret = "upstream-secret-in-throw";
  globalThis.fetch = async () => {
    throw new Error(secret);
  };
  try {
    const res = await worker.fetch(
      req("/v1/generate", {
        task_id: "failure",
        provider: "workers-ai",
        model: "@cf/m",
        prompt: "p",
        upload_url: "https://media.test/out",
      }),
      env,
    );
    assert.equal(res.status, 500);
    const text = await res.text();
    assert.equal(JSON.parse(text).error.code, "ADAPTER_INTERNAL_ERROR");
    assert.ok(!text.includes(secret));
  } finally {
    globalThis.fetch = previousFetch;
  }
});

test("invalid upstream image bytes never upload", async () => {
  let puts = 0;
  const res = await handleGenerate(
    req("/v1/generate", {
      task_id: "invalid-bytes",
      provider: "workers-ai",
      model: "@cf/m",
      prompt: "p",
      upload_url: "https://media.test/out",
    }),
    env,
    async (_url, init) => {
      if (init?.method === "PUT") puts++;
      return new Response("<svg></svg>", {
        headers: { "content-type": "image/png" },
      });
    },
  );
  assert.equal(res.status, 500);
  assert.equal(puts, 0);
});
