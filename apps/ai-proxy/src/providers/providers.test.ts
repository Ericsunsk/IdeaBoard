import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { prepareRoute } from "./index.js";
import { UnsupportedModelError, UpstreamError } from "./shared.js";
import { maskSecrets } from "../io.js";
import type { CanonicalImageRequest, Env } from "../contracts.js";
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

describe("route selection through the prepared target", () => {
  const cases = [
    {
      name: "google provider",
      provider: "google",
      model: "nano-banana",
      suffix: "/ai/run",
      bodyModel: "google/nano-banana",
      source: "airun:google:text-to-image",
    },
    {
      name: "openai gpt-image",
      provider: "openai",
      model: "gpt-image-1",
      suffix: "/ai/run",
      bodyModel: "openai/gpt-image-1",
      source: "airun:openai:gpt-image",
    },
    {
      name: "openai non-gpt-image",
      provider: "openai",
      model: "dall-e-3",
      suffix: "/openai/images/generations",
      bodyModel: "dall-e-3",
      source: "openai:images:generations",
    },
    {
      name: "krea provider",
      provider: "krea",
      model: "krea-3",
      suffix: "/ai/run",
      bodyModel: "krea/krea-3",
      source: "airun:krea",
    },
    {
      name: "fal provider",
      provider: "fal",
      model: "fal-ai/flux",
      suffix: "/fal/fal-ai/flux",
      source: "fal:text-to-image",
    },
    {
      name: "workers-ai migration",
      provider: "workers-ai",
      model: "@cf/m",
      suffix: "/ai/run",
      bodyModel: "@cf/m",
      source: "airun:migrated:workers-ai:text-to-image",
    },
    {
      name: "pool declaration beats model prefix",
      provider: "fal",
      model: "google/nano-banana",
      suffix: "/fal/google/nano-banana",
      source: "fal:text-to-image",
    },
    {
      name: "custom provider model prefix",
      provider: "custom-acme",
      model: "google/nano-banana",
      suffix: "/ai/run",
      bodyModel: "google/nano-banana",
      source: "airun:google:text-to-image",
    },
  ];
  for (const c of cases) {
    test(c.name, () => {
      const route = prepareRoute(
        { task_id: "route", prompt: "p", provider: c.provider, model: c.model },
        env,
        [],
      );
      assert.ok(route.target.url.endsWith(c.suffix));
      assert.equal(route.target.path_source, c.source);
      if (c.bodyModel)
        assert.equal(
          JSON.parse(route.target.body as string).model,
          c.bodyModel,
        );
      if (c.provider === "workers-ai")
        assert.match(JSON.stringify(route.target.dryBody), /migration_note/);
    });
  }
  test("unknown route fails before sending", () => {
    assert.throws(
      () =>
        prepareRoute(
          { task_id: "route", prompt: "p", provider: "nope", model: "nothing" },
          env,
          [],
        ),
      UnsupportedModelError,
    );
  });
});

describe("airun google target (/ai/run envelope)", () => {
  test("text-to-image envelope + host + CF token auth", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google",
        model: "nano-banana",
        prompt: "hi",
        upload_url: "https://s3/x",
      },
      env,
      [],
    ).target;
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
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "google",
        model: "google/nano-banana-2",
        prompt: "p",
        upload_url: "https://s3/x",
      },
      env,
      [fakeInput(0)],
    ).target;
    assert.equal(t.path_source, "airun:google:img2img");
    const body = JSON.parse(t.body as string) as {
      model: string;
      input: Record<string, unknown>;
    };
    assert.equal(body.model, "google/nano-banana-2");
    assert.equal(body.input["aspect_ratio"], "match_input_image");
    assert.ok(
      Array.isArray(body.input["image_input"]) &&
        (body.input["image_input"] as string[])[0].startsWith("data:image/"),
    );
    const dry = JSON.stringify(t.dryBody);
    assert.ok(!dry.includes("data:image/"));
    assert.ok(dry.includes("<input-image:0:redacted>"));
  });

  test("unknown provider_options rejected (additionalProperties:false)", () => {
    assert.throws(
      () =>
        prepareRoute(
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
        ).target,
      UnsupportedModelError,
    );
  });

  test("invalid enum + over-limit inputs fail closed", () => {
    assert.throws(
      () =>
        prepareRoute(
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
        ).target,
      UnsupportedModelError,
    );
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "t",
            provider: "google",
            model: "nano-banana",
            prompt: "p",
            upload_url: "https://s3/x",
          },
          env,
          [fakeInput(0), fakeInput(1), fakeInput(2), fakeInput(3)],
        ).target,
      UnsupportedModelError,
    );
  });
});

describe("airun openai target (gpt-image)", () => {
  test("text-to-image: images omitted, size from width/height", () => {
    const t = prepareRoute(
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
    ).target;
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
    const t = prepareRoute(
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
    ).target;
    assert.equal(t.path_source, "airun:openai:gpt-image:edit");
    const body = JSON.parse(t.body as string) as {
      input: Record<string, unknown>;
    };
    assert.ok((body.input["images"] as string[])[0].startsWith("data:image/"));
    assert.equal(body.input["quality"], "high");
    assert.equal(body.input["background"], "opaque");
  });
});

describe("airun migrated target (workers-ai legacy)", () => {
  test("@cf model via /ai/run + gateway id + dry-run note", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "workers-ai",
        model: "@cf/model/x",
        prompt: "hi",
        upload_url: "https://s3/x",
      },
      env,
      [],
    ).target;
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
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "fal",
        model: "fal-ai/fast-sdxl",
        prompt: "cat",
        upload_url: "https://s3/x",
      },
      env,
      [fakeInput(0)],
    ).target;
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
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "openai",
        model: "dall-e-3",
        prompt: "p",
        upload_url: "https://s3/x",
      },
      env,
      [],
    ).target;
    assert.ok(t.url.endsWith("/openai/images/generations"));
    assert.equal(t.path_source, "openai:images:generations");
  });

  test("openai with images -> edits multipart", () => {
    const t = prepareRoute(
      {
        task_id: "t",
        provider: "openai",
        model: "dall-e-2",
        mask_index: 1,
        prompt: "p",
        upload_url: "https://s3/x",
      },
      env,
      [fakeInput(0), fakeInput(1)],
    ).target;
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
    const out = await prepareRoute(
      {
        task_id: "extract",
        prompt: "p",
        provider: "workers-ai",
        model: "@cf/m",
      },
      env,
      [],
    ).extractImage(res);
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
    const out = await prepareRoute(
      {
        task_id: "extract",
        prompt: "p",
        provider: "google",
        model: "nano-banana",
      },
      env,
      [],
    ).extractImage(res, fetchFn);
    assert.equal(out.bytes.byteLength, 4);
    assert.equal(out.seed, 42);
  });

  test("airun base64 result decoded natively", async () => {
    const raw = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
    const b64 = Buffer.from(raw).toString("base64");
    const res = new Response(JSON.stringify({ result: { image: b64 } }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    const out = await prepareRoute(
      {
        task_id: "extract",
        prompt: "p",
        provider: "openai",
        model: "gpt-image-1",
      },
      env,
      [],
    ).extractImage(res);
    assert.equal(out.bytes.byteLength, 8);
  });

  test("airun queue payload fail-closed UNSUPPORTED_MODEL", async () => {
    const res = new Response(
      JSON.stringify({ request_id: "q1", status: "IN_QUEUE" }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
    await assert.rejects(
      () =>
        prepareRoute(
          {
            task_id: "extract",
            prompt: "p",
            provider: "krea",
            model: "krea-3",
          },
          env,
          [],
        ).extractImage(res),
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
    await assert.rejects(
      () =>
        prepareRoute(
          { task_id: "extract", prompt: "p", provider: "fal", model: "flux" },
          env,
          [],
        ).extractImage(res),
      UnsupportedModelError,
    );
  });

  test("fal 202 fail-closed", async () => {
    const res = new Response(
      JSON.stringify({ request_id: "q1", status: "IN_PROGRESS" }),
      {
        status: 202,
        headers: { "content-type": "application/json" },
      },
    );
    await assert.rejects(
      () =>
        prepareRoute(
          { task_id: "extract", prompt: "p", provider: "fal", model: "flux" },
          env,
          [],
        ).extractImage(res),
      UnsupportedModelError,
    );
  });

  test("openai gateway missing data fail-closed UPSTREAM", async () => {
    const res = new Response(JSON.stringify({ data: [] }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    await assert.rejects(
      () =>
        prepareRoute(
          {
            task_id: "extract",
            prompt: "p",
            provider: "openai",
            model: "dall-e-3",
          },
          env,
          [],
        ).extractImage(res),
      UpstreamError,
    );
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
    test(`${file} imports only contracts/io/shared`, () => {
      const src = readFileSync(file, "utf8");
      const imports = [...src.matchAll(/from\s+["']([^"']+)["']/g)].map(
        (m) => m[1],
      );
      for (const spec of imports) {
        assert.ok(
          spec === "../../contracts.js" ||
            spec === "../../io.js" ||
            spec === "../shared.js",
          `${file} must not import ${spec}`,
        );
      }
    });
  }
});

describe("explicit image roles", () => {
  const req = {
    task_id: "roles",
    provider: "fal",
    model: "flux",
    prompt: "test",
  };
  test("Fal keeps every unmasked reference and only splits an explicit mask", () => {
    const inputs = [fakeInput(1), fakeInput(2), fakeInput(3)];
    const plain = JSON.parse(
      prepareRoute(req, env, inputs).target.body as string,
    );
    assert.deepEqual(
      plain.image_urls,
      inputs.map((i) => i.dataUri),
    );
    assert.equal(plain.mask_url, undefined);
    const masked = JSON.parse(
      prepareRoute({ ...req, mask_index: 1 }, env, inputs).target
        .body as string,
    );
    assert.deepEqual(masked.image_urls, [inputs[0].dataUri, inputs[2].dataUri]);
    assert.equal(masked.mask_url, inputs[1].dataUri);
  });
  test("OpenAI edits refuses unsupported multiple references instead of dropping them", () => {
    const r = { ...req, provider: "openai", model: "dall-e-2" };
    assert.throws(
      () => prepareRoute(r, env, [fakeInput(1), fakeInput(2)]).target,
      UnsupportedModelError,
    );
    const target = prepareRoute({ ...r, mask_index: 1 }, env, [
      fakeInput(1),
      fakeInput(2),
    ]).target;
    assert.ok(target.body instanceof FormData);
    assert.ok(target.body.get("mask") instanceof Blob);
  });
  test("Airun preserves references or rejects unsupported roles", () => {
    const inputs = [fakeInput(1), fakeInput(2), fakeInput(3)];
    for (const provider of ["google", "openai", "krea"]) {
      const r = {
        ...req,
        provider,
        model: provider === "openai" ? "gpt-image-1" : "m",
      };
      const body = JSON.parse(
        prepareRoute(r, env, inputs).target.body as string,
      ).input;
      assert.deepEqual(
        body.image_input ?? body.images ?? body.image_urls,
        inputs.map((i) => i.dataUri),
      );
      assert.throws(
        () => prepareRoute({ ...r, mask_index: 2 }, env, inputs).target,
        UnsupportedModelError,
      );
    }
    const migrated = { ...req, provider: "workers-ai", model: "@cf/m" };
    assert.throws(
      () => prepareRoute(migrated, env, inputs).target,
      UnsupportedModelError,
    );
    const body = JSON.parse(
      prepareRoute({ ...migrated, mask_index: 1 }, env, inputs.slice(0, 2))
        .target.body as string,
    ).input;
    assert.equal(body.image_b64, inputs[0].base64);
    assert.equal(body.mask_b64, inputs[1].base64);
  });
});

test("base64 extractors preserve JPEG/WebP output bytes", async () => {
  for (const raw of [
    [255, 216, 255, 224, 0],
    [...Buffer.from("RIFF0000WEBPVP8 ")],
  ]) {
    const b64 = Buffer.from(raw).toString("base64");
    for (const model of ["gpt-image-1", "dall-e-3"]) {
      const route = prepareRoute(
        { task_id: "mime", prompt: "p", provider: "openai", model },
        env,
        [],
      );
      const out = await route.extractImage(
        new Response(JSON.stringify({ data: [{ b64_json: b64 }] })),
      );
      assert.deepEqual(new Uint8Array(out.bytes), new Uint8Array(raw));
    }
  }
});

test("provider_options cannot smuggle image roles", () => {
  for (const provider of ["fal", "openai", "workers-ai", "krea", "google"]) {
    assert.throws(
      () =>
        prepareRoute(
          {
            task_id: "roles",
            provider,
            model: provider === "workers-ai" ? "@cf/m" : "m",
            prompt: "p",
            provider_options: { mask_url: "https://secret.test/mask" },
          },
          env,
          [fakeInput(1)],
        ).target,
      UnsupportedModelError,
    );
  }
});

describe("prepared route preserves request/response pairing", () => {
  for (const c of [
    { provider: "workers-ai", model: "@cf/m", expected: "/ai/run" },
    {
      provider: "openai",
      model: "dall-e-3",
      expected: "/openai/images/generations",
    },
    { provider: "fal", model: "flux", expected: "/fal/flux" },
  ]) {
    test(
      c.provider +
        " keeps its decoder after request identity and diagnostic labels change",
      async () => {
        const req: CanonicalImageRequest = {
          task_id: "paired",
          prompt: "p",
          provider: c.provider,
          model: c.model,
        };
        const route = prepareRoute(req, env, []);
        const png = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
        const b64 = Buffer.from(png).toString("base64");
        const response =
          c.provider === "workers-ai"
            ? new Response(png, { headers: { "content-type": "image/png" } })
            : new Response(
                JSON.stringify(
                  c.provider === "fal"
                    ? { images: [{ url: "https://result.test/image" }] }
                    : { data: [{ b64_json: b64 }] },
                ),
                { headers: { "content-type": "application/json" } },
              );
        req.provider = "unrecognized";
        req.model = "changed-after-preparation";
        route.target.path_source = "editorial-label-only";
        let downloads = 0;
        const fetchFn = (async (url: RequestInfo | URL) => {
          downloads++;
          assert.equal(String(url), "https://result.test/image");
          return new Response(png, {
            headers: { "content-type": "image/png" },
          });
        }) as typeof fetch;
        assert.ok(route.target.url.endsWith(c.expected));
        const out = await route.extractImage(response, fetchFn);
        assert.deepEqual(new Uint8Array(out.bytes), png);
        assert.equal(downloads, c.provider === "fal" ? 1 : 0);
      },
    );
  }

  test("OpenAI JSON and multipart operations stay independent of diagnostic labels", async () => {
    for (const inputs of [[], [fakeInput(0)]]) {
      const route = prepareRoute(
        {
          task_id: "operation",
          prompt: "p",
          provider: "openai",
          model: "dall-e-2",
        },
        env,
        inputs,
      );
      route.target.path_source = "renamed-diagnostic";
      assert.equal(route.target.body instanceof FormData, inputs.length > 0);
      assert.ok(
        route.target.url.endsWith(
          inputs.length ? "/images/edits" : "/images/generations",
        ),
      );
      assert.equal(
        route.target.headers["Content-Type"],
        inputs.length ? undefined : "application/json",
      );
      const png = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
      const out = await route.extractImage(
        new Response(
          JSON.stringify({
            data: [{ b64_json: Buffer.from(png).toString("base64") }],
          }),
        ),
      );
      assert.deepEqual(new Uint8Array(out.bytes), png);
    }
  });
});
