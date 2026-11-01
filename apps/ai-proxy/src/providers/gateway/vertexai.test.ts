import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { prepareRoute } from "../index.js";
import { UnsupportedModelError, UpstreamError } from "../shared.js";
import type { Env } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";

const env: Env = {
  CF_ACCOUNT_ID: "acct123",
  CF_GATEWAY_ID: "gw1",
  CF_AIG_TOKEN: "cf-token",
  WORKER_INTERNAL_SECRET: "s3cr3t",
};

function fakeInput(i: number): FetchedInput {
  const bytes = new Uint8Array([137, 80, 78, 71, i]).buffer as ArrayBuffer;
  const b64 = Buffer.from(bytes).toString("base64");
  return {
    bytes,
    contentType: "image/png",
    base64: b64,
    dataUri: `data:image/png;base64,${b64}`,
  };
}

const BASE_OPTS = {
  vertex_project: "proj-1",
  vertex_region: "us-central1",
};

function pngB64(raw: number[]): string {
  return Buffer.from(raw).toString("base64");
}

describe("vertex-ai URL 组装（channel 层 transport）", () => {
  test("全参：project/region/resource 进 URL", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "gemini-2.0-flash-001",
        prompt: "cat",
        provider_options: {
          ...BASE_OPTS,
          vertex_resource: "generateContent",
        },
      },
      env,
      [],
    ).target;
    assert.equal(
      t.url,
      "https://gateway.ai.cloudflare.com/v1/acct123/gw1/google-vertex-ai/v1/projects/proj-1/locations/us-central1/publishers/google/models/gemini-2.0-flash-001:generateContent",
    );
    assert.equal(t.method, "POST");
    assert.equal(t.path_source, "gateway:vertex-ai:generateContent");
    assert.equal(t.headers["cf-aig-authorization"], "Bearer cf-token");
    assert.equal(t.headers["Content-Type"], "application/json");
  });

  test("缺 project 直接 fail-closed（generate 前抛）", () => {
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex-ai",
            model: "gemini-2.0-flash-001",
            prompt: "p",
            provider_options: { vertex_region: "us-central1" },
          },
          env,
          [],
        ).target,
      UnsupportedModelError,
    );
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex-ai",
            model: "gemini-2.0-flash-001",
            prompt: "p",
          },
          env,
          [],
        ).target,
      UnsupportedModelError,
    );
  });

  test("缺 region 用 us-central1 缺省", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "gemini-2.0-flash-001",
        prompt: "p",
        provider_options: { vertex_project: "proj-1" },
      },
      env,
      [],
    ).target;
    assert.ok(t.url.includes("/locations/us-central1/"), t.url);
  });

  test("缺 resource 缺省 generateContent；未知 resource fail-closed", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "gemini-2.0-flash-001",
        prompt: "p",
        provider_options: { vertex_project: "proj-1" },
      },
      env,
      [],
    ).target;
    assert.ok(t.url.endsWith(":generateContent"), t.url);
    assert.equal(t.path_source, "gateway:vertex-ai:generateContent");
    for (const bad of ["streamGenerateContent", "publish", "", "PREDICT"]) {
      assert.throws(
        () =>
          prepareRoute(
            {
              task_id: "t",
              provider: "google-vertex-ai",
              model: "m",
              prompt: "p",
              provider_options: {
                vertex_project: "proj-1",
                vertex_resource: bad,
              },
            },
            env,
            [],
          ).target,
        UnsupportedModelError,
      );
    }
  });

  test("变体 slug google-vertex 不进 gateway（fail-closed）", () => {
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex",
            model: "m",
            prompt: "p",
          },
          env,
          [],
        ),
      UnsupportedModelError,
    );
  });
});

describe("vertex-ai generateContent input 映射", () => {
  test("无图：contents 单文本 part", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "gemini-2.0-flash-001",
        prompt: "a cat",
        provider_options: { ...BASE_OPTS },
      },
      env,
      [],
    ).target;
    const body = JSON.parse(t.body as string) as {
      contents: Array<{ role: string; parts: Array<{ text?: string }> }>;
    };
    assert.equal(body.contents[0].role, "user");
    assert.deepEqual(body.contents[0].parts, [{ text: "a cat" }]);
  });

  test("有图：append inlineData part，dry-run 脱敏", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "gemini-2.0-flash-001",
        prompt: "edit",
        provider_options: { ...BASE_OPTS },
      },
      env,
      [fakeInput(0), fakeInput(1)],
    ).target;
    const body = JSON.parse(t.body as string) as {
      contents: Array<{ parts: Array<Record<string, unknown>> }>;
    };
    assert.equal(body.contents[0].parts.length, 3);
    const img = body.contents[0].parts[1] as {
      inlineData: { mimeType: string; data: string };
    };
    assert.equal(img.inlineData.mimeType, "image/png");
    assert.ok(img.inlineData.data.length > 0);
    const dry = JSON.stringify(t.dryBody);
    assert.ok(!dry.includes(img.inlineData.data));
    assert.ok(dry.includes("<input-image:0:redacted>"));
    assert.ok(dry.includes("<input-image:1:redacted>"));
  });

  test("explicit native options survive while transport keys and contents stay owned", () => {
    const generationConfig = {
      responseModalities: ["TEXT", "IMAGE"],
      imageConfig: { aspectRatio: "16:9" },
    };
    const safetySettings = [
      {
        category: "HARM_CATEGORY_HATE_SPEECH",
        threshold: "BLOCK_MEDIUM_AND_ABOVE",
      },
    ];
    const input = fakeInput(2);
    const target = prepareRoute(
      {
        task_id: "options",
        provider: "google-vertex-ai",
        model: "gemini-2.5-flash-image",
        prompt: "canonical prompt",
        provider_options: {
          ...BASE_OPTS,
          vertex_resource: "generateContent",
          generationConfig,
          safetySettings,
          contents: [{ role: "user", parts: [{ text: "override" }] }],
          model: "override",
        },
      },
      env,
      [input],
    ).target;
    const body = JSON.parse(target.body as string);
    const dry = target.dryBody as typeof body;
    for (const value of [body, dry]) {
      assert.deepEqual(value.generationConfig, generationConfig);
      assert.deepEqual(value.safetySettings, safetySettings);
      assert.equal(value.contents[0].parts[0].text, "canonical prompt");
      for (const key of [
        "vertex_project",
        "vertex_region",
        "vertex_resource",
        "model",
      ])
        assert.ok(!(key in value));
    }
    assert.equal(body.contents[0].parts[1].inlineData.data, input.base64);
    assert.equal(
      dry.contents[0].parts[1].inlineData.data,
      "<input-image:0:redacted>",
    );
    assert.ok(!JSON.stringify(dry).includes(input.base64));
  });

  test("无 prompt fail-closed（有图也不放行）；mask 拒绝", () => {
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex-ai",
            model: "m",
            prompt: "",
            provider_options: { ...BASE_OPTS },
          },
          env,
          [fakeInput(0)],
        ).target,
      UnsupportedModelError,
    );
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex-ai",
            model: "m",
            prompt: "p",
            mask_index: 0,
            provider_options: { ...BASE_OPTS },
          },
          env,
          [fakeInput(0), fakeInput(1)],
        ).target,
      UnsupportedModelError,
    );
  });
});

describe("vertex-ai predict input 映射（Imagen）", () => {
  test("parameters 透传：显式 options + seed/宽高，不猜缺省", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "imagen-3.0-generate-002",
        prompt: "a cat",
        seed: 7,
        width: 1024,
        height: 768,
        provider_options: {
          ...BASE_OPTS,
          vertex_resource: "predict",
          sampleCount: 1,
          aspectRatio: "1:1",
        },
      },
      env,
      [],
    ).target;
    assert.ok(t.url.endsWith(":predict"), t.url);
    assert.equal(t.path_source, "gateway:vertex-ai:predict");
    const body = JSON.parse(t.body as string) as {
      instances: Array<{ prompt: string }>;
      parameters: Record<string, unknown>;
    };
    assert.equal(body.instances[0].prompt, "a cat");
    assert.equal(body.parameters["sampleCount"], 1);
    assert.equal(body.parameters["aspectRatio"], "1:1");
    assert.equal(body.parameters["seed"], 7);
    assert.equal(body.parameters["width"], 1024);
    assert.equal(body.parameters["height"], 768);
    // transport 保留键不进 parameters。
    assert.ok(!("vertex_project" in body.parameters));
    assert.ok(!("vertex_region" in body.parameters));
    assert.ok(!("vertex_resource" in body.parameters));
  });

  test("有图走 instances.image.bytesBase64Encoded；多图/mask 拒绝", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google-vertex-ai",
        model: "imagen-3.0-generate-002",
        prompt: "p",
        provider_options: { ...BASE_OPTS, vertex_resource: "predict" },
      },
      env,
      [fakeInput(3)],
    ).target;
    const body = JSON.parse(t.body as string) as {
      instances: Array<{ image?: { bytesBase64Encoded: string } }>;
    };
    assert.equal(
      body.instances[0].image?.bytesBase64Encoded,
      fakeInput(3).base64,
    );
    const dry = JSON.stringify(t.dryBody);
    assert.ok(!dry.includes(fakeInput(3).base64));
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex-ai",
            model: "m",
            prompt: "p",
            provider_options: { ...BASE_OPTS, vertex_resource: "predict" },
          },
          env,
          [fakeInput(0), fakeInput(1)],
        ).target,
      UnsupportedModelError,
    );
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google-vertex-ai",
            model: "m",
            prompt: "p",
            mask_index: 0,
            provider_options: { ...BASE_OPTS, vertex_resource: "predict" },
          },
          env,
          [fakeInput(0), fakeInput(1)],
        ).target,
      UnsupportedModelError,
    );
  });
});

describe("vertex-ai 回包三形态（录制 fixture，不外发）", () => {
  const png = [137, 80, 78, 71, 13, 10, 26, 10];
  const b64 = pngB64(png);

  function route(resource: string) {
    return prepareRoute(
      {
        task_id: "x",
        provider: "google-vertex-ai",
        model: "m",
        prompt: "p",
        provider_options: { ...BASE_OPTS, vertex_resource: resource },
      },
      env,
      [],
    );
  }

  test("generateContent inlineData（camel + snake 兼容）", async () => {
    for (const part of [
      { inlineData: { mimeType: "image/png", data: b64 } },
      { inline_data: { mime_type: "image/png", data: b64 } },
    ]) {
      const out = await route("generateContent").extractImage(
        new Response(
          JSON.stringify({
            candidates: [
              { content: { role: "model", parts: [{ text: "ok" }, part] } },
            ],
          }),
        ),
      );
      assert.deepEqual(new Uint8Array(out.bytes), new Uint8Array(png));
    }
  });

  test("generateContent 纯文本无图 -> UpstreamError", async () => {
    await assert.rejects(
      () =>
        route("generateContent").extractImage(
          new Response(
            JSON.stringify({
              candidates: [{ content: { parts: [{ text: "only text" }] } }],
            }),
          ),
        ),
      UpstreamError,
    );
  });

  test("predict 顶层与嵌套 bytesBase64Encoded 兼容", async () => {
    for (const pred of [
      { bytesBase64Encoded: b64 },
      { image: { bytesBase64Encoded: b64 } },
      b64,
    ]) {
      const out = await route("predict").extractImage(
        new Response(JSON.stringify({ predictions: [pred] })),
      );
      assert.deepEqual(new Uint8Array(out.bytes), new Uint8Array(png));
    }
  });

  test("predict 空 predictions -> UpstreamError；queue -> UnsupportedModelError", async () => {
    await assert.rejects(
      () =>
        route("predict").extractImage(
          new Response(JSON.stringify({ predictions: [] })),
        ),
      UpstreamError,
    );
    await assert.rejects(
      () =>
        route("predict").extractImage(
          new Response(JSON.stringify({ predictions: [{}] })),
        ),
      UpstreamError,
    );
    await assert.rejects(
      () =>
        route("generateContent").extractImage(
          new Response(JSON.stringify({ request_id: "q", status: "IN_QUEUE" })),
        ),
      UnsupportedModelError,
    );
  });
});

describe("vertex-ai dry-run parity", () => {
  test("generate 与 dry-run 同 URL/path_source，headers 永不返回", async () => {
    const { handleDryRun } = await import("../../handler.js");
    const gen = prepareRoute(
      {
        task_id: "d",
        provider: "google-vertex-ai",
        model: "gemini-2.0-flash-001",
        prompt: "p",
        provider_options: { vertex_project: "proj-1" },
      },
      env,
      [],
    ).target;
    const res = await handleDryRun(
      new Request("https://edge.test/v1/dry-run", {
        method: "POST",
        headers: {
          Authorization: "Bearer s3cr3t",
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          task_id: "d",
          provider: "google-vertex-ai",
          model: "gemini-2.0-flash-001",
          prompt: "p",
          provider_options: { vertex_project: "proj-1" },
        }),
      }),
      env,
    );
    assert.equal(res.status, 200);
    const body = (await res.json()) as {
      target: { url: string; method: string; body: unknown };
      path_source: string;
    };
    assert.equal(body.target.url, gen.url);
    assert.equal(body.target.method, "POST");
    assert.equal(body.path_source, gen.path_source);
    assert.ok(!("headers" in body.target));
  });
});
