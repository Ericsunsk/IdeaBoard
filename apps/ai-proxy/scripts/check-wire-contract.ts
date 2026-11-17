import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import worker from "../src/index.js";
import type { CanonicalImageRequest, Env } from "../src/contracts.js";

// Go owns its DTOs; exercise their real JSON encoding against the real Edge
// entry, then send actual Edge responses back through the Go decoders.
const apiRoot = fileURLToPath(new URL("../../api/", import.meta.url));
const fixturePath = fileURLToPath(
  new URL(
    "../../api/internal/platform/workerwire/testdata/requests.json",
    import.meta.url,
  ),
);
const goArgs = ["run", "./internal/platform/workerwire/testdata/contract"];
const encoded = JSON.parse(
  execFileSync("go", [...goArgs, "encode", fixturePath], {
    cwd: apiRoot,
    encoding: "utf8",
  }),
) as {
  requests: {
    full: CanonicalImageRequest;
    minimal: CanonicalImageRequest;
    inherited: CanonicalImageRequest;
    probe: CanonicalImageRequest;
  };
  jpeg: string;
};
const { full, minimal, inherited, probe } = encoded.requests;
const jpeg = Buffer.from(encoded.jpeg, "base64");
const env: Env = {
  CF_ACCOUNT_ID: "contract-account",
  CF_GATEWAY_ID: "contract-gateway",
  CF_AIG_TOKEN: "contract-token",
  WORKER_INTERNAL_SECRET: "contract-secret",
};
const upstreamURL =
  "https://api.cloudflare.com/client/v4/accounts/contract-account/ai/run";
const responses: Record<string, { status: number; body: unknown }> = {};
const originalFetch = globalThis.fetch;
const noFetch: typeof fetch = async () => {
  throw new Error("this contract case must not fetch");
};

async function execute(
  name: string,
  body: CanonicalImageRequest,
  fetchFn: typeof fetch,
  path = "/v1/generate",
  auth = `Bearer ${env.WORKER_INTERNAL_SECRET}`,
) {
  let fetchFailure: unknown;
  globalThis.fetch = async (...args) => {
    // Native data-URL decoding stays local; never delegate HTTP(S).
    if (String(args[0]).startsWith("data:")) return originalFetch(...args);
    try {
      return await fetchFn(...args);
    } catch (err) {
      fetchFailure = err;
      throw err;
    }
  };
  try {
    const response = await worker.fetch(
      new Request(`https://edge.test${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: auth },
        body: JSON.stringify(body),
      }),
      env,
    );
    if (fetchFailure && name !== "internal") throw fetchFailure;
    responses[name] = {
      status: response.status,
      body: await response.json(),
    };
  } finally {
    globalThis.fetch = originalFetch;
  }
}

const mask = Buffer.from([1, 2, 3]);
const reference = Buffer.from([4, 5, 6]);
for (const [name, request] of [
  ["full", full],
  ["minimal", minimal],
] as const) {
  let upstreams = 0;
  let uploads = 0;
  let inputs = 0;
  await execute(name, request, async (url, init) => {
    if (String(url) === full.images?.[0]) {
      inputs++;
      return new Response(mask, { headers: { "content-type": "image/png" } });
    }
    if (String(url) === full.images?.[1]) {
      inputs++;
      return new Response(reference, {
        headers: { "content-type": "image/png" },
      });
    }
    if (String(url) === upstreamURL) {
      upstreams++;
      assert.equal(init?.method, "POST");
      const upstream = JSON.parse(String(init?.body));
      assert.equal(upstream.model, request.model);
      assert.equal(upstream.input.prompt, request.prompt);
      if (name === "full") {
        assert.deepEqual(upstream.input, {
          prompt: "contract prompt",
          negative_prompt: "no blur",
          width: 768,
          height: 512,
          num_steps: 0,
          guidance: 0,
          seed: 0,
          contract_option: "kept",
          image_b64: reference.toString("base64"),
          mask_b64: mask.toString("base64"),
        });
        return Response.json(
          { result: { image: encoded.jpeg, seed: 0 } },
          {
            headers: {
              "cf-aig-log-id": "contract-log",
              "cf-aig-cache-status": "MISS",
            },
          },
        );
      }
      assert.deepEqual(upstream.input, {
        prompt: "minimal prompt",
        width: 1024,
        height: 1024,
        num_steps: 20,
        guidance: 3.5,
      });
      return new Response(jpeg, { headers: { "content-type": "image/jpeg" } });
    }
    assert.equal(String(url), request.upload_url, "unexpected external fetch");
    assert.equal(init?.method, "PUT");
    assert.equal(new Headers(init?.headers).get("content-type"), "image/jpeg");
    assert.deepEqual(Buffer.from(init?.body as ArrayBuffer), jpeg);
    uploads++;
    return new Response(null, { status: 200 });
  });
  assert.equal(responses[name].status, 200);
  assert.equal(upstreams, 1);
  assert.equal(uploads, 1);
  assert.equal(inputs, name === "full" ? 2 : 0);
}

let inheritedInputs = 0;
let inheritedUpstreams = 0;
let inheritedUploads = 0;
await execute("inherited", inherited, async (url, init) => {
  if (String(url) === inherited.images?.[0]) {
    inheritedInputs++;
    return new Response(jpeg, { headers: { "content-type": "image/jpeg" } });
  }
  if (String(url).includes("/custom-gwllm/v1/images/edits")) {
    inheritedUpstreams++;
    const form = await new Request(String(url), init).formData();
    assert.equal(form.get("size"), "925x1024");
    assert.equal(form.get("model"), inherited.model);
    assert.equal(form.get("n"), "1");
    assert.deepEqual(
      Buffer.from(await (form.get("image") as Blob).arrayBuffer()),
      jpeg,
    );
    return Response.json({ data: [{ b64_json: encoded.jpeg }] });
  }
  assert.equal(String(url), inherited.upload_url, "unexpected external fetch");
  assert.equal(init?.method, "PUT");
  assert.deepEqual(Buffer.from(init?.body as ArrayBuffer), jpeg);
  inheritedUploads++;
  return new Response(null, { status: 200 });
});
assert.equal(responses.inherited.status, 200);
assert.equal(inheritedInputs, 1);
assert.equal(inheritedUpstreams, 1);
assert.equal(inheritedUploads, 1);

let dryFetches = 0;
await execute(
  "probe",
  probe,
  async () => {
    dryFetches++;
    throw new Error("dry-run must not fetch");
  },
  "/v1/dry-run",
);
assert.equal(dryFetches, 0);
assert.equal(probe.images, undefined);
assert.equal(probe.upload_url, undefined);
assert.equal(probe.public_url, undefined);
assert.equal(responses.probe.status, 200);
assert.ok(!JSON.stringify(responses.probe).includes("contract-private"));
assert.ok(JSON.stringify(responses.probe).includes('"contract_option":"kept"'));

await execute("bad_request", { ...minimal, upload_url: undefined }, noFetch);
await execute("unauthorized", minimal, noFetch, "/v1/generate", "Bearer wrong");
await execute("upstream", minimal, async (url) => {
  assert.equal(String(url), upstreamURL);
  return new Response("contract upstream failure", { status: 503 });
});
await execute("unsupported", minimal, async (url) => {
  assert.equal(String(url), upstreamURL);
  return new Response(null, { status: 202 });
});
await execute("input_fetch", full, async (url) => {
  assert.ok(full.images?.includes(String(url)));
  return new Response(null, { status: 403 });
});
await execute("upload", minimal, async (url) => {
  if (String(url) === upstreamURL) {
    return new Response(jpeg, { headers: { "content-type": "image/jpeg" } });
  }
  assert.equal(String(url), minimal.upload_url);
  return new Response(null, { status: 403 });
});
await execute("internal", minimal, async (url) => {
  assert.equal(String(url), upstreamURL);
  throw new Error("contract adapter failure");
});

process.stdout.write(
  execFileSync("go", [...goArgs, "verify"], {
    cwd: apiRoot,
    input: JSON.stringify(responses),
    encoding: "utf8",
  }),
);
console.log("Worker wire contract passed; no network or storage writes");
