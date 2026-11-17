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
  });
});

test("GWLLM single-reference edit uses multipart bytes with browser-set boundary", () => {
  const target = prepareRoute(req, env, [input]).target;
  assert.equal(target.url, `${gatewayUrl}edits`);
  assert.equal(target.headers["Content-Type"], undefined);
  assert.ok(target.body instanceof FormData);
  assert.equal(target.body.get("model"), "gpt-image-1");
  assert.equal(target.body.get("prompt"), req.prompt);
  assert.ok(target.body.get("image") instanceof Blob);
  assert.equal(target.body.get("mask"), null);
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
