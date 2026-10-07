const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const script = html.match(/<script>\s*([\s\S]*?)<\/script>/)[1];

function bootEnvironment() {
  const classes = new Set();
  const elements = {
    boot: { classList: { add: value => classes.add(value), remove: value => classes.delete(value) } },
    status: { className: "", textContent: "" },
    spinner: { style: {} },
  };
  const frames = [];
  const context = vm.createContext({
    document: { getElementById: id => elements[id] },
    WebAssembly: { instantiateStreaming: async () => ({ instance: {} }) },
    fetch: async () => ({}),
    requestAnimationFrame: callback => frames.push(callback),
    console: { error() {}, warn() {}, log() {} },
  });
  return {
    elements,
    context,
    hidden: () => classes.has("hidden"),
    render: () => {
      while (frames.length) frames.shift()();
    },
  };
}

function startBoot(run) {
  const boot = bootEnvironment();
  boot.context.Go = class {
    constructor() { this.importObject = {}; this.exit = () => {}; }
    run() { return run(this); }
  };
  boot.completion = vm.runInContext(script, boot.context);
  return boot;
}

function startGoExit(immediate) {
  const boot = bootEnvironment();
  const goRoot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8", windowsHide: true }).trim();
  const shim = fs.readFileSync(path.join(goRoot, "lib", "wasm", "wasm_exec.js"), "utf8");
  Object.assign(boot.context, { crypto, performance, TextEncoder, TextDecoder });
  vm.runInContext(shim, boot.context);
  const RuntimeGo = boot.context.Go;
  let runtime;
  boot.context.Go = class extends RuntimeGo {
    constructor() { super(); runtime = this; }
  };
  class SyntheticInstance {}
  boot.context.WebAssembly.Instance = SyntheticInstance;
  boot.context.WebAssembly.instantiateStreaming = async (_, imports) => {
    const memory = new WebAssembly.Memory({ initial: 1 });
    const exit = () => {
      new DataView(memory.buffer).setInt32(8, 2, true);
      imports.gojs["runtime.wasmExit"](0);
    };
    const instance = new SyntheticInstance();
    instance.exports = { mem: memory, run: () => { if (immediate) exit(); }, resume: exit };
    return { instance };
  };
  boot.exit = () => runtime._resume();
  boot.completion = vm.runInContext(script, boot.context);
  return boot;
}

test("startup failure remains visible after pending animation callbacks", async () => {
  const boot = startBoot(() => { throw new Error("synthetic startup failure"); });
  await boot.completion;
  boot.render();
  assert.equal(boot.hidden(), false);
  assert.match(boot.elements.status.textContent, /synthetic startup failure/);
  assert.equal(boot.elements.spinner.style.display, "none");
});

test("runtime failure restores the already hidden error surface", async () => {
  let failRuntime;
  const runtime = new Promise((_, reject) => { failRuntime = reject; });
  const boot = startBoot(() => runtime);
  await new Promise(resolve => setImmediate(resolve));
  boot.render();
  assert.equal(boot.hidden(), true);
  failRuntime(new Error("synthetic runtime failure"));
  await boot.completion;
  assert.equal(boot.hidden(), false);
  assert.match(boot.elements.status.textContent, /synthetic runtime failure/);
});

test("a running runtime reveals the canvas", async () => {
  let finishRuntime;
  const runtime = new Promise(resolve => { finishRuntime = resolve; });
  const boot = startBoot(() => runtime);
  await new Promise(resolve => setImmediate(resolve));
  boot.render();
  assert.equal(boot.hidden(), true);
  assert.notEqual(boot.elements.status.className, "err");
  finishRuntime();
  await boot.completion;
});

test("Go nonzero exit remains visible after pending animation callbacks", async () => {
  const boot = startGoExit(true);
  await boot.completion;
  boot.render();
  assert.equal(boot.hidden(), false);
  assert.match(boot.elements.status.textContent, /code 2/);
});

test("Go nonzero exit restores the already hidden error surface", async () => {
  const boot = startGoExit(false);
  await new Promise(resolve => setImmediate(resolve));
  boot.render();
  assert.equal(boot.hidden(), true);
  boot.exit();
  await boot.completion;
  assert.equal(boot.hidden(), false);
  assert.match(boot.elements.status.textContent, /code 2/);
});
