import { test } from "node:test";
import assert from "node:assert/strict";
import type { CanonicalImageRequest, Env } from "../../contracts.js";
import { handleDryRun, handleGenerate } from "../../handler.js";
import type { FetchedInput } from "../../io.js";
import { prepareRoute } from "../index.js";
import { UnsupportedModelError } from "../shared.js";

const env: Env = {
  CF_ACCOUNT_ID: "account",
  CF_GATEWAY_ID: "gateway",
  CF_AIG_TOKEN: "gateway-token",
  WORKER_INTERNAL_SECRET: "internal-secret",
};
const req: CanonicalImageRequest = {
  task_id: "gwllm-test",
  provider: "custom-gwllm",
  model: "gpt-image-1",
  prompt: "A bicycle",
  upload_url: "https://storage.test/output?signature=private",
};

test("configured GWLLM identity and published references share one route", () => {
  for (const provider of [
    "gwllm",
    "custom-gwllm",
    "workers-ai",
    "unknown-provider",
  ]) {
    const models =
      provider === "workers-ai" || provider === "unknown-provider"
        ? [
            "gwllm/Vendor/Case-Sensitive-Model",
            "custom-gwllm/Vendor/Case-Sensitive-Model",
          ]
        : [
            "Vendor/Case-Sensitive-Model",
            "gwllm/Vendor/Case-Sensitive-Model",
            "custom-gwllm/Vendor/Case-Sensitive-Model",
          ];
    for (const model of models) {
      const target = prepareRoute({ ...req, provider, model }, env, []).target;
      assert.equal(target.url, `${gatewayUrl}generations`);
      assert.equal(
        JSON.parse(target.body as string).model,
        "Vendor/Case-Sensitive-Model",
      );
    }
  }
  for (const [model, nativeModel] of [
    ["gwllm/custom-gwllm/Vendor/Model", "custom-gwllm/Vendor/Model"],
    ["custom-gwllm/gwllm/Vendor/Model", "gwllm/Vendor/Model"],
  ]) {
    const target = prepareRoute(
      { ...req, provider: "gwllm", model },
      env,
      [],
    ).target;
    assert.equal(JSON.parse(target.body as string).model, nativeModel);
  }
});
const png = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
const base64 = Buffer.from(png).toString("base64");
const input: FetchedInput = {
  bytes: png.buffer,
  contentType: "image/png",
  base64,
  dataUri: `data:image/png;base64,${base64}`,
};
const gatewayUrl =
  "https://gateway.ai.cloudflare.com/v1/account/gateway/custom-gwllm/v1/images/";

function request(body: CanonicalImageRequest): Request {
  return new Request("https://worker.test/v1/generate", {
    method: "POST",
    headers: { Authorization: "Bearer internal-secret" },
    body: JSON.stringify(body),
  });
}

test("GWLLM uses CF BYOK, preserves model identity and maps generation options", () => {
  const target = prepareRoute(
    {
      ...req,
      model: "custom-gwllm/Vendor/Case-Sensitive-Model",
      width: 1536,
      height: 1024,
      provider_options: { quality: "high", response_format: "url" },
    },
    env,
    [],
  ).target;
  assert.equal(target.url, `${gatewayUrl}generations`);
  assert.equal(target.headers["cf-aig-authorization"], "Bearer gateway-token");
  assert.equal(target.headers["cf-aig-skip-cache"], "true");
  assert.equal(target.headers["cf-aig-collect-log-payload"], "false");
  assert.ok(
    !Object.keys(target.headers).some(
      (k) => k.toLowerCase() === "authorization",
    ),
  );
  assert.deepEqual(JSON.parse(target.body as string), {
    model: "Vendor/Case-Sensitive-Model",
    prompt: "A bicycle",
    size: "1536x1024",
    response_format: "url",
    quality: "high",
    n: 1,
  });
});

test("GWLLM rejects multiple outputs before sending a billable request", async () => {
  for (const n of [0, 2, 4, "1", null]) {
    let calls = 0;
    const body = { ...req, provider_options: { n } };
    const response = await handleGenerate(request(body), env, async () => {
      calls++;
      return Response.json({ data: [{ b64_json: base64 }] });
    });
    assert.equal(response.status, 422);
    assert.equal(calls, 0);
    const preview = await handleDryRun(request(body), env);
    assert.equal(preview.status, 400);
  }
});

test("GWLLM single-reference edit uses multipart bytes with browser-set boundary", () => {
  const target = prepareRoute(req, env, [input]).target;
  assert.equal(target.url, `${gatewayUrl}edits`);
  assert.equal(target.headers["Content-Type"], undefined);
  assert.ok(target.body instanceof FormData);
  assert.equal(target.body.get("model"), "gpt-image-1");
  assert.equal(target.body.get("prompt"), req.prompt);
  assert.equal(target.body.get("n"), "1");
  assert.ok(target.body.get("image") instanceof Blob);
  assert.equal(target.body.get("mask"), null);
});

test("GWLLM generation and edits honor canonical size and the same provider options", async () => {
  for (const count of [0, 1, 4]) {
    const body = {
      ...req,
      width: 2048,
      height: 3072,
      ...(count
        ? { images: Array(count).fill("https://input.test/image") }
        : {}),
      provider_options: {
        size: "1024x1024",
        response_format: "url",
        quality: "high",
        n: 1,
      },
    };
    let fields: Record<string, unknown> | undefined;
    const response = await handleGenerate(
      request(body),
      env,
      async (url, init) => {
        if (String(url) === "https://input.test/image")
          return new Response(png, {
            headers: { "Content-Type": "image/png" },
          });
        if (String(url).startsWith(gatewayUrl)) {
          fields = count
            ? Object.fromEntries(
                await new Request(String(url), init).formData(),
              )
            : JSON.parse(init?.body as string);
          return Response.json({
            data: [{ url: "https://result.test/image" }],
          });
        }
        if (String(url) === "https://result.test/image")
          return new Response(png);
        assert.equal(String(url), req.upload_url);
        return new Response(null);
      },
    );
    assert.equal(response.status, 200);
    assert.ok(fields);
    assert.equal(fields.size, "2048x3072");
    assert.equal(fields.response_format, "url");
    assert.equal(fields.quality, "high");
    assert.equal(fields.n, count ? "1" : 1);
  }
});

test("GWLLM multipart preview shows serialized provider fields while redacting nested secrets", async () => {
  const body = {
    ...req,
    images: ["https://input.test/image"],
    provider_options: {
      size: "auto",
      response_format: "url",
      quality: "high",
      seed: 0,
      metadata: { label: "preview", api_key: "nested-private" },
      auth: "top-level-private",
    },
  };
  const target = prepareRoute(body, env, [input]).target;
  const wire = await new Request(target.url, {
    method: target.method,
    headers: target.headers,
    body: target.body,
  }).formData();
  const response = await handleDryRun(request(body), env);
  assert.equal(response.status, 200);
  const preview = (await response.json()) as {
    target: { body: { fields: Record<string, unknown> } };
  };
  const fields = preview.target.body.fields;
  assert.equal(wire.get("size"), "auto");
  for (const key of [
    "model",
    "prompt",
    "size",
    "response_format",
    "quality",
    "seed",
    "n",
  ])
    assert.equal(fields[key], wire.get(key));
  assert.equal(fields.auth, "***");
  assert.deepEqual(JSON.parse(fields.metadata as string), {
    label: "preview",
    api_key: "***",
  });
  assert.ok(!JSON.stringify(preview).includes("private"));
  assert.equal(wire.get("auth"), "top-level-private");
});

test("GWLLM field policy preserves existing OpenAI generation and edit overrides", async () => {
  const body = {
    ...req,
    provider: "openai",
    model: "dall-e-3",
    width: 1536,
    height: 1024,
    provider_options: {
      model: "configured-model",
      prompt: "configured-prompt",
      size: "auto",
      response_format: "url",
      quality: "high",
      n: 3,
    },
  };
  const generation = prepareRoute(body, env, []).target;
  assert.deepEqual(
    JSON.parse(generation.body as string),
    body.provider_options,
  );
  const edit = prepareRoute(body, env, [input]).target;
  const form = await new Request(edit.url, {
    method: edit.method,
    headers: edit.headers,
    body: edit.body,
  }).formData();
  assert.equal(form.get("model"), "dall-e-3");
  assert.equal(form.get("prompt"), "A bicycle");
  assert.equal(form.get("size"), "1536x1024");
  assert.equal(form.get("response_format"), "b64_json");
  assert.equal(form.get("quality"), "high");
  assert.equal(form.get("n"), "3");
  assert.equal(form.getAll("image").length, 1);
});

test("GWLLM sends every ordered reference as multipart image[] through generate", async () => {
  for (const model of ["gpt-image-2.6-flare", "nano-banana-pro"]) {
    for (const count of [2, 3, 4]) {
      const references = Array.from({ length: count }, (_, index) => ({
        url: `https://input.test/${index}?signature=private`,
        bytes: new Uint8Array([...png, index]),
      }));
      const fetched: string[] = [];
      let edits = 0;
      let uploads = 0;
      const fetchFn: typeof fetch = async (url, init) => {
        const reference = references.find((item) => item.url === String(url));
        if (reference) {
          fetched.push(reference.url);
          return new Response(reference.bytes, {
            headers: { "Content-Type": "image/png" },
          });
        }
        if (String(url) === `${gatewayUrl}edits`) {
          edits++;
          assert.equal(init?.method, "POST");
          assert.ok(init.body instanceof FormData);
          // Serialize and parse the wire body to catch lost/replaced repeated fields.
          const form = await new Request(String(url), init).formData();
          assert.equal(form.get("model"), model);
          assert.equal(form.get("prompt"), req.prompt);
          assert.equal(form.get("image"), null);
          assert.equal(form.get("mask"), null);
          const files = form.getAll("image[]");
          assert.equal(files.length, count);
          for (const [index, file] of files.entries()) {
            assert.ok(file instanceof Blob);
            assert.equal(file.type, "image/png");
            assert.deepEqual(
              new Uint8Array(await file.arrayBuffer()),
              references[index].bytes,
            );
          }
          assert.ok(
            !JSON.stringify([...form.entries()]).includes("signature=private"),
          );
          return Response.json({ data: [{ b64_json: base64 }] });
        }
        assert.equal(String(url), req.upload_url);
        assert.equal(init?.method, "PUT");
        assert.deepEqual(new Uint8Array(init?.body as ArrayBuffer), png);
        uploads++;
        return new Response(null);
      };
      const res = await handleGenerate(
        request({ ...req, model, images: references.map((item) => item.url) }),
        env,
        fetchFn,
      );
      assert.equal(res.status, 200);
      assert.deepEqual(
        fetched,
        references.map((item) => item.url),
      );
      assert.equal(edits, 1);
      assert.equal(uploads, 1);
    }
  }
});

test("GWLLM rejects unsupported roles, streaming and canonical-field overrides", () => {
  assert.throws(
    () => prepareRoute({ ...req, model: "custom-gwllm/" }, env, []),
    UnsupportedModelError,
  );
  assert.throws(
    () => prepareRoute(req, env, Array(5).fill(input)),
    UnsupportedModelError,
  );
  assert.throws(
    () => prepareRoute({ ...req, mask_index: 1 }, env, [input, input]),
    UnsupportedModelError,
  );
  for (const provider_options of [
    { model: "different-billable-model" },
    { prompt: "different-prompt" },
    { stream: true },
    { image: "https://untrusted.test/image" },
    { "image[]": ["https://untrusted.test/image"] },
    { mask_url: "https://untrusted.test/mask" },
  ]) {
    assert.throws(
      () => prepareRoute({ ...req, provider_options }, env, []),
      UnsupportedModelError,
    );
  }
});

test("GWLLM dry-run redacts secrets and image bytes without fetching", async () => {
  const res = await handleDryRun(
    request({
      ...req,
      images: Array.from(
        { length: 4 },
        (_, index) => `https://input.test/${index}?signature=private`,
      ),
    }),
    env,
  );
  assert.equal(res.status, 200);
  const body = await res.text();
  const preview = JSON.parse(body) as {
    target: { body: { fields: Record<string, unknown> } };
  };
  assert.deepEqual(
    preview.target.body.fields["image[]"],
    Array.from(
      { length: 4 },
      (_, index) => `<input-image:${index}:bytes-redacted>`,
    ),
  );
  assert.ok(body.includes(`${gatewayUrl}edits`));
  assert.ok(body.includes("bytes-redacted"));
  for (const secret of [
    "gateway-token",
    "internal-secret",
    "signature=private",
    base64,
  ]) {
    assert.ok(!body.includes(secret));
  }
  assert.ok(!body.includes('"headers"'));
});

test("GWLLM generate decodes base64 or URL output and uploads actual bytes", async () => {
  for (const image of [
    { b64_json: base64 },
    { url: "https://result.test/image" },
  ]) {
    const calls: string[] = [];
    const fetchFn: typeof fetch = async (url, init) => {
      calls.push(String(url));
      if (String(url) === `${gatewayUrl}generations`) {
        assert.equal(init?.method, "POST");
        return Response.json(
          { data: [image] },
          { headers: { "cf-aig-log-id": "log-1" } },
        );
      }
      if (String(url) === "https://result.test/image") return new Response(png);
      assert.equal(String(url), req.upload_url);
      assert.equal(init?.method, "PUT");
      assert.equal(
        (init?.headers as Record<string, string>)["Content-Type"],
        "image/png",
      );
      assert.deepEqual(new Uint8Array(init?.body as ArrayBuffer), png);
      return new Response(null, { status: 200 });
    };
    const res = await handleGenerate(request(req), env, fetchFn);
    assert.equal(res.status, 200);
    const body = (await res.json()) as {
      provider: string;
      content_type: string;
      receipt: { log_id: string };
    };
    assert.equal(body.provider, "custom-gwllm");
    assert.equal(body.content_type, "image/png");
    assert.equal(body.receipt.log_id, "log-1");
    assert.equal(calls.length, "url" in image ? 3 : 2);
  }
});

test("GWLLM unusable success and async responses never upload or poll", async () => {
  for (const [payload, status, expected] of [
    [{ error: { message: "rejected" } }, 200, 502],
    [{ request_id: "queued", status: "PENDING" }, 200, 422],
    [{ request_id: "queued" }, 202, 422],
  ] as const) {
    let calls = 0;
    const res = await handleGenerate(request(req), env, async () => {
      calls++;
      return Response.json(payload, { status });
    });
    assert.equal(res.status, expected);
    assert.equal(calls, 1);
  }
});
