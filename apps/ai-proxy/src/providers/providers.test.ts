import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  UnsupportedModelError,
  buildTarget,
  extractImage,
  resolveChannel,
} from "./index.js";
import { extractAirunImage } from "./airun/index.js";
import {
  extractFalImage,
  extractOpenAiImage,
} from "./gateway/index.js";
import { buildFalTarget, buildOpenAiTarget } from "./gateway/index.js";
import { maskSecrets } from "../io.js";
import type { Env } from "../contracts.js";
import type { FetchedInput } from "../io.js";

const env: Env = {
  CF_ACCOUNT_ID: "acct123",
  CF_GATEWAY_ID: "gw1",
  CF_AIG_TOKEN: "cf-token",
  INTERNAL_SECRET: "s3cr3t",
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

describe("dispatch rules (channel first, vendor second)", () => {
  test("google provider -> airun/google, model prefixed", () => {
    const r = resolveChannel({ provider: "google", model: "nano-banana" });
    assert.equal(r.channel, "airun");
    assert.equal(r.vendor, "google");
    assert.equal(r.model, "google/nano-banana");
    assert.equal(r.migrated, undefined);
  });

  test("openai gpt-image -> airun/openai", () => {
    const r = resolveChannel({ provider: "openai", model: "gpt-image-1" });
    assert.equal(r.channel, "airun");
    assert.equal(r.vendor, "openai");
    assert.equal(r.model, "openai/gpt-image-1");
  });

  test("openai non-gpt-image -> gateway/openai", () => {
    const r = resolveChannel({ provider: "openai", model: "dall-e-3" });
    assert.equal(r.channel, "gateway");
    assert.equal(r.vendor, "openai");
  });

  test("krea provider -> airun generic", () => {
    const r = resolveChannel({ provider: "krea", model: "krea-3" });
    assert.equal(r.channel, "airun");
    assert.equal(r.vendor, "krea");
    assert.equal(r.model, "krea/krea-3");
  });

  test("fal provider -> gateway/fal", () => {
    const r = resolveChannel({ provider: "fal", model: "fal-ai/flux" });
    assert.equal(r.channel, "gateway");
    assert.equal(r.vendor, "fal");
  });

  test("workers-ai legacy slug -> airun migrated", () => {
    const r = resolveChannel({ provider: "workers-ai", model: "@cf/m" });
    assert.equal(r.channel, "airun");
    assert.equal(r.vendor, "migrated");
    assert.equal(r.migrated, true);
  });

  test("pool declaration wins over model prefix", () => {
    const r = resolveChannel({
      provider: "fal",
      model: "google/nano-banana",
    });
    assert.equal(r.channel, "gateway");
    assert.equal(r.vendor, "fal");
  });

  test("custom provider falls back to model prefix", () => {
    const r = resolveChannel({
      provider: "custom-acme",
      model: "google/nano-banana",
    });
    assert.equal(r.channel, "airun");
    assert.equal(r.vendor, "google");
  });

  test("unknown provider/model -> UnsupportedModelError", () => {
    assert.throws(
      () => resolveChannel({ provider: "nope", model: "nothing" }),
      UnsupportedModelError,
    );
  });
});

describe("airun google target (/ai/run envelope)", () => {
  test("text-to-image envelope + host + CF token auth", () => {
    const t = buildTarget(
      {
        task_id: "t",
        provider: "google",
        model: "nano-banana",
        prompt: "hi",
        upload_url: "https://s3/x",
      },
      env,
      [],
    );
    assert.equal(
      t.url,
      "https://api.cloudflare.com/client/v4/accounts/acct123/ai/run",
    );
    assert.equal(t.method, "POST");
    assert.equal(t.path_source, "airun:google:text-to-image");
    assert.match(t.headers["Authorization"], /Bearer cf-token/);
    const body = JSON.parse(t.body as string) as {
      model: string;
      input: Record<string, unknown>;
    };
    assert.equal(body.model, "google/nano-banana");
    assert.equal(body.input["prompt"], "hi");
    assert.equal(body.input["aspect_ratio"], "1:1");
    assert.equal(body.input["output_format"], "png");
    assert.equal(body.input["resolution"], "1K");
    assert.ok(!("image_input" in body.input));
  });

  test("img2img defaults match_input_image + data URI, dry redacted", () => {
    const t = buildTarget(
      {
        task_id: "t",
        provider: "google",
        model: "google/nano-banana-2",
        prompt: "p",
        upload_url: "https://s3/x",
      },
      env,
      [fakeInput(0)],
    );
    assert.equal(t.path_source, "airun:google:img2img");
    const body = JSON.parse(t.body as string) as {
      model: string;
      input: Record<string, unknown>;
    };
    assert.equal(body.model, "google/nano-banana-2");
    assert.equal(body.input["aspect_ratio"], "match_input_image");
    assert.ok(
      Array.isArray(body.input["image_input"]) &&
        ((body.input["image_input"] as string[])[0].startsWith("data:image/")),
    );
    const dry = JSON.stringify(t.dryBody);
    assert.ok(!dry.includes("data:image/"));
    assert.ok(dry.includes("<input-image:0:redacted>"));
  });

  test("unknown provider_options rejected (additionalProperties:false)", () => {
    assert.throws(
      () =>
        buildTarget(
          {
            task_id: "t",
            provider: "google",
            model: "nano-banana",
            prompt: "p",
            upload_url: "https://s3/x",
            provider_options: { width: 512 },
          },
          env,
          [],
        ),
      UnsupportedModelError,
    );
  });

  test("invalid enum + over-limit inputs fail closed", () => {
    assert.throws(
      () =>
        buildTarget(
          {
            task_id: "t",
            provider: "google",
            model: "nano-banana",
            prompt: "p",
            upload_url: "https://s3/x",
            provider_options: { aspect_ratio: "99:99" },
          },
          env,
          [],
        ),
      UnsupportedModelError,
    );
    assert.throws(
      () =>
        buildTarget(
          {
            task_id: "t",
            provider: "google",
            model: "nano-banana",
            prompt: "p",
            upload_url: "https://s3/x",
          },
          env,
          [fakeInput(0), fakeInput(1), fakeInput(2), fakeInput(3)],
        ),
      UnsupportedModelError,
    );
  });
});

describe("airun openai target (gpt-image)", () => {
  test("text-to-image: images omitted, size from width/height", () => {
    const t = buildTarget(
      {
        task_id: "t",
        provider: "openai",
        model: "gpt-image-1",
        prompt: "p",
        width: 512,
        height: 768,
        upload_url: "https://s3/x",
      },
      env,
      [],
    );
    assert.equal(
      t.url,
      "https://api.cloudflare.com/client/v4/accounts/acct123/ai/run",
    );
    assert.equal(t.path_source, "airun:openai:gpt-image");
    const body = JSON.parse(t.body as string) as {
      model: string;
      input: Record<string, unknown>;
    };
    assert.equal(body.model, "openai/gpt-image-1");
    assert.equal(body.input["size"], "512x768");
    assert.ok(!("images" in body.input));
  });

  test("edit: images data URIs + quality passthrough (Merge)", () => {
    const t = buildTarget(
      {
        task_id: "t",
        provider: "openai",
        model: "gpt-image-1",
        prompt: "p",
        upload_url: "https://s3/x",
        provider_options: { quality: "high", background: "opaque" },
      },
      env,
      [fakeInput(0)],
    );
    assert.equal(t.path_source, "airun:openai:gpt-image:edit");
    const body = JSON.parse(t.body as string) as {
      input: Record<string, unknown>;
    };
    assert.ok(
      ((body.input["images"] as string[])[0].startsWith("data:image/")),
    );
    assert.equal(body.input["quality"], "high");
    assert.equal(body.input["background"], "opaque");
  });
});

describe("airun migrated target (workers-ai legacy)", () => {
  test("@cf model via /ai/run + gateway id + dry-run note", () => {
    const t = buildTarget(
      {
        task_id: "t",
        provider: "workers-ai",
        model: "@cf/model/x",
        prompt: "hi",
        upload_url: "https://s3/x",
      },
      env,
      [],
    );
    assert.equal(
      t.url,
      "https://api.cloudflare.com/client/v4/accounts/acct123/ai/run",
    );
    assert.equal(t.headers["cf-aig-gateway-id"], "gw1");
    assert.equal(t.path_source, "airun:migrated:workers-ai:text-to-image");
    const body = JSON.parse(t.body as string) as { model: string };
    assert.equal(body.model, "@cf/model/x");
    const dry = JSON.stringify(t.dryBody);
    assert.ok(dry.includes("migration_note"));
  });
});

describe("gateway regression (behavior unchanged)", () => {
  test("fal sync endpoint only, never queue target", () => {
    const t = buildFalTarget(
      {
        task_id: "t",
        provider: "fal",
        model: "fal-ai/fast-sdxl",
        prompt: "cat",
        upload_url: "https://s3/x",
      },
      env,
      [fakeInput(0)],
    );
    assert.equal(
      t.url,
      "https://gateway.ai.cloudflare.com/v1/acct123/gw1/fal/fal-ai/fast-sdxl",
    );
    assert.equal(t.headers["x-fal-target-url" as never], undefined);
    assert.ok(!t.url.includes("queue."));
    assert.equal(t.path_source, "fal:img2img");
    const body = JSON.parse(t.body as string) as Record<string, unknown>;
    assert.ok(
      typeof body["image_url"] === "string" &&
        (body["image_url"] as string).startsWith("data:image/"),
    );
  });

  test("openai no images -> generations", () => {
    const t = buildOpenAiTarget(
      {
        task_id: "t",
        provider: "openai",
        model: "dall-e-3",
        prompt: "p",
        upload_url: "https://s3/x",
      },
      env,
      [],
    );
    assert.ok(t.url.endsWith("/openai/images/generations"));
    assert.equal(t.path_source, "openai:images:generations");
  });

  test("openai with images -> edits multipart", () => {
    const t = buildOpenAiTarget(
      {
        task_id: "t",
        provider: "openai",
        model: "dall-e-2",
        prompt: "p",
        upload_url: "https://s3/x",
      },
      env,
      [fakeInput(0), fakeInput(1)],
    );
    assert.ok(t.url.endsWith("/openai/images/edits"));
    assert.equal(t.path_source, "openai:images:edits");
    assert.ok(t.body instanceof FormData);
    assert.ok((t.dryBody as { kind: string }).kind.includes("multipart"));
  });
});

describe("extractors (recorded fixtures, no network)", () => {
  test("airun binary passthrough", async () => {
    const bytes = new Uint8Array([1, 2, 3]).buffer as ArrayBuffer;
    const res = new Response(bytes, {
      status: 200,
      headers: { "content-type": "image/png" },
    });
    const out = await extractAirunImage(res);
    assert.equal(out.contentType, "image/png");
    assert.equal(out.bytes.byteLength, 3);
  });

  test("airun presigned URL result fetched", async () => {
    const png = new Uint8Array([137, 80, 78, 71]).buffer as ArrayBuffer;
    const fetchFn = (async () =>
      new Response(png, {
        status: 200,
        headers: { "content-type": "image/png" },
      })) as typeof fetch;
    const res = new Response(
      JSON.stringify({
        result: { image: "https://r2.example/out.png", seed: 42 },
        state: "Completed",
      }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
    const out = await extractImage(
      { provider: "google", model: "nano-banana" },
      res,
      fetchFn,
    );
    assert.equal(out.contentType, "image/png");
    assert.equal(out.bytes.byteLength, 4);
    assert.equal(out.seed, 42);
  });

  test("airun base64 result decoded natively", async () => {
    const raw = new Uint8Array([1, 2, 3, 4]);
    const b64 = Buffer.from(raw).toString("base64");
    const res = new Response(JSON.stringify({ result: { image: b64 } }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    const out = await extractImage(
      { provider: "openai", model: "gpt-image-1" },
      res,
    );
    assert.equal(out.contentType, "image/png");
    assert.equal(out.bytes.byteLength, 4);
  });

  test("airun queue payload fail-closed UNSUPPORTED_MODEL", async () => {
    const res = new Response(
      JSON.stringify({ request_id: "q1", status: "IN_QUEUE" }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
    await assert.rejects(
      () => extractImage({ provider: "krea", model: "krea-3" }, res),
      UnsupportedModelError,
    );
  });

  test("fal queue payload fail-closed UNSUPPORTED_MODEL", async () => {
    const res = new Response(
      JSON.stringify({
        request_id: "q1",
        status: "IN_QUEUE",
        queue_position: 3,
      }),
      {
        status: 200,
        headers: { "content-type": "application/json" },
      },
    );
    await assert.rejects(() => extractFalImage(res), UnsupportedModelError);
  });

  test("fal 202 fail-closed", async () => {
    const res = new Response(
      JSON.stringify({ request_id: "q1", status: "IN_PROGRESS" }),
      {
        status: 202,
        headers: { "content-type": "application/json" },
      },
    );
    await assert.rejects(() => extractFalImage(res), UnsupportedModelError);
  });

  test("openai gateway missing data fail-closed UPSTREAM", async () => {
    const res = new Response(JSON.stringify({ data: [] }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    const { UpstreamError } = await import("./index.js");
    await assert.rejects(() => extractOpenAiImage(res), UpstreamError);
  });
});

describe("maskSecrets", () => {
  test("redacts token/key/secret/auth", () => {
    const out = JSON.stringify(
      maskSecrets({
        api_key: "k",
        nested: { authToken: "t", ok: 1 },
        list: [{ secret: "s" }],
      }),
    );
    assert.ok(
      !out.includes('"k"') && !out.includes('"t"') && !out.includes('"s"'),
    );
    assert.ok(out.includes("***"));
  });
});

describe("iron rule: vendor files are input-only", () => {
  const vendors = [
    "src/providers/airun/google.ts",
    "src/providers/airun/openai.ts",
    "src/providers/gateway/fal.ts",
    "src/providers/gateway/openai.ts",
  ];
  const banned = [
    "gateway.ai.cloudflare.com",
    "api.cloudflare.com",
    "cf-aig-authorization",
    "cf-aig-gateway-id",
    "CF_AIG_TOKEN",
    "CF_ACCOUNT_ID",
    "CF_GATEWAY_ID",
    "gatewayBase",
    "airunBase",
    "AIRUN_BASE",
    "CF_GATEWAY_BASE",
    "PROVIDER_SLUG",
    "OPENAI_GENERATIONS_PATH",
    "OPENAI_EDITS_PATH",
    "Authorization",
  ];
  for (const file of vendors) {
    test(`${file} has no transport tokens`, () => {
      const src = readFileSync(file, "utf8");
      for (const token of banned) {
        assert.ok(!src.includes(token), `${file} must not contain ${token}`);
      }
    });
    test(`${file} imports only contracts/io/root`, () => {
      const src = readFileSync(file, "utf8");
      const imports = [...src.matchAll(/from\s+["']([^"']+)["']/g)].map(
        (m) => m[1],
      );
      for (const spec of imports) {
        assert.ok(
          spec === "../../contracts.js" ||
            spec === "../../io.js" ||
            spec === "../index.js",
          `${file} must not import ${spec}`,
        );
      }
    });
  }
});
