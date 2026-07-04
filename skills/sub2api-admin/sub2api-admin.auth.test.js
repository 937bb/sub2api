const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const http = require("node:http");
const path = require("node:path");
const test = require("node:test");

const cliPath = path.join(__dirname, "scripts", "sub2api-admin.js");
const cliArgs = ["api", "GET", "/admin/groups/all"];

function cleanEnv(extra = {}) {
  const env = {
    NODE_DISABLE_COLORS: "1",
  };
  for (const key of ["PATH", "Path", "SystemRoot", "ComSpec", "TEMP", "TMP", "HOME", "USERPROFILE"]) {
    if (process.env[key]) env[key] = process.env[key];
  }
  return { ...env, ...extra };
}

function runCli({ baseUrl, apiKey, jwt }) {
  const env = cleanEnv({
    SUB2API_BASE_URL: baseUrl,
    ...(apiKey !== undefined ? { SUB2API_ADMIN_API_KEY: apiKey } : {}),
    ...(jwt !== undefined ? { SUB2API_JWT: jwt } : {}),
  });

  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [cliPath, ...cliArgs], {
      env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";

    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("close", (code, signal) => {
      resolve({ code, signal, stdout, stderr });
    });
  });
}

async function withRecordingServer(fn) {
  const requests = [];
  const server = http.createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      requests.push({
        method: req.method,
        url: req.url,
        headers: req.headers,
      });
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ code: 0, data: { ok: true } }));
    });
  });

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });

  try {
    const { port } = server.address();
    return await fn({
      baseUrl: `http://127.0.0.1:${port}`,
      requests,
    });
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
  }
}

test("API key auth sends x-api-key without Authorization", async () => {
  await withRecordingServer(async ({ baseUrl, requests }) => {
    const result = await runCli({ baseUrl, apiKey: "admin-key" });

    assert.equal(result.code, 0, result.stderr);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].method, "GET");
    assert.equal(requests[0].url, "/api/v1/admin/groups/all");
    assert.equal(requests[0].headers["x-api-key"], "admin-key");
    assert.equal(requests[0].headers.authorization, undefined);
  });
});

test("JWT auth sends Authorization bearer without x-api-key", async () => {
  await withRecordingServer(async ({ baseUrl, requests }) => {
    const result = await runCli({ baseUrl, jwt: "jwt-token" });

    assert.equal(result.code, 0, result.stderr);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].headers.authorization, "Bearer jwt-token");
    assert.equal(requests[0].headers["x-api-key"], undefined);
  });
});

test("API key wins when API key and JWT are both set", async () => {
  await withRecordingServer(async ({ baseUrl, requests }) => {
    const result = await runCli({ baseUrl, apiKey: "admin-key", jwt: "jwt-token" });

    assert.equal(result.code, 0, result.stderr);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].headers["x-api-key"], "admin-key");
    assert.equal(requests[0].headers.authorization, undefined);
  });
});

test("missing API key and JWT exits before making a request", async () => {
  await withRecordingServer(async ({ baseUrl, requests }) => {
    const result = await runCli({ baseUrl });

    assert.notEqual(result.code, 0);
    assert.equal(requests.length, 0);
    assert.match(result.stderr, /Missing SUB2API_ADMIN_API_KEY or SUB2API_JWT/);
  });
});
