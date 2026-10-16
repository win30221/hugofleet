import * as k from "./vendor/anatomy/iso-kit.mjs";

// Six Figure-size assemblies made from the same machined bench. Every mechanism
// is a mounted slab or a carriage on rails; there are no tubes or turned solids.
const WIDTH = 880, HEIGHT = 470;
const BASE = { x: -210, y: -106, w: 428, d: 228, r: 8 };
export const projection = k.fitProjection(k.boxCorners(BASE, -22, 108), WIDTH, HEIGHT, { pad: 44, azimuth: 52, elevation: 32 });
const P = projection;
const S = (plan, z, h, options = {}) => k.solidSvg(k.slabOf(plan, z, h, P, 4, Math.min(0.7, h / 4)), options);
const plan = (x, y, w, d, r = 1) => ({ x, y, w, d, r });
const L = (d, tone = "faint") => k.lineSvg(d, { tone });
const bolts = (p, z, inset = 4) => k.dotsSvg(k.corners(p, inset).map(([x, y]) => k.iso([x, y, z], P)), { size: 0.7, tone: "hi" });
const sorted = (points) => [...points].sort((a, b) => k.depthOf(a, P) - k.depthOf(b, P));
const group = (name, body, extra = "") => `<g data-part="${name}" ${extra}>${body}</g>`;

function bench() {
  const out = [`<path class="iso-halo" d="${k.haloOf(BASE, -22, 1, P)}"/>`];
  for (const [x, y] of sorted(k.corners(BASE, 24).map(([x, y]) => [x, y, -22]))) {
    out.push(S(plan(x - 8, y - 8, 16, 16, 3), -22, 8, { tone: "lo" }));
  }
  out.push(S(BASE, -14, 14, { tone: "mid", inner: L(k.sideSeam(BASE, -7, P, 4)) }));
  out.push(L(k.planOutline(k.insetPlan(BASE, 6), 0.1, P, 4)), bolts(BASE, 0.2, 12));
  const strip = plan(-183, 108, 360, 7);
  out.push(S(strip, 0, 0.6, { tone: "lo" }));
  const ticks = k.topTicks(-178, 172, 5, 5, 108.5, 0.6, [2, 4], P);
  out.push(L(ticks.minor, "lo"), L(ticks.major, "mid"));
  // Closed ventilation slots have no floating line ends.
  for (let i = 0; i < 9; i++) out.push(L(k.planOutline(plan(-194 + i * 5, -91, 2, 18), 0.1, P, 3), "lo"));
  return out.join("");
}

function consoleUnit(chapter) {
  const body = plan(-181, -70, 105, 93, 5);
  const out = [S(body, 0, 6, { tone: "mid" }), S(k.insetPlan(body, 3), 6, 47, { tone: "mid" })];
  // Four toe clamps bolt the console to the bench rather than floating on it.
  for (const x of [-185, -80]) for (const y of [-61, 12]) out.push(S(plan(x, y, 8, 7), 0, 4, { tone: "mid" }));
  const top = k.insetPlan(body, 1.5);
  out.push(S(top, 53, 3, { tone: "hi" }), bolts(top, 56));
  const glass = plan(-166, -57, 72, 46, 3);
  out.push(S(glass, 56, 1.2, { tone: "mid", className: "display" }));
  // Closed engraved traces on the display, rooted to the display surface.
  for (let i = 0; i < 4; i++) out.push(L(k.planOutline(plan(-158, -49 + i * 8, 50 - i * 7, 2), 57.25, P, 2), "mid"));
  for (let i = 0; i < 3; i++) out.push(S(plan(-163 + i * 22, 4, 15, 10, 2), 56, 2, { tone: "mid" }));
  const keyboard = plan(-172, 28, 88, 24, 3);
  out.push(S(keyboard, 0, 4, { tone: "mid" }));
  for (let row = 0; row < 2; row++) for (let key = 0; key < 7; key++) out.push(L(k.planOutline(plan(-166 + key * 11, 32 + row * 8, 7, 5), 4.1, P, 2), "mid"));
  if (chapter >= 2) {
    const cards = [];
    for (let i = 0; i < 3; i++) cards.push(S(plan(-180, -97, 96, 20, 2), i * 4, 4, { tone: "mid" }));
    const receipt = S(plan(-171, -94, 76, 14, 1), 12, 1, { tone: "hi", lit: true });
    out.unshift(group("ledger", cards.join("") + group("receipt", receipt)));
  }
  const template = S(plan(-161, -7, 65, 25, 2), 59, 1.2, { tone: "hi", lit: true });
  out.push(group("template", template));
  return group("console", out.join(""));
}

function pod(x, index, worker, y = -49, depth = 60, z = 42) {
  const casePlan = plan(x, y, 31, depth, 2);
  const out = [S(casePlan, z, 6, { tone: "hi", lit: true })];
  out.push(L(k.planOutline(k.insetPlan(casePlan, 2), z + 6.1, P, 3), "lo"), bolts(casePlan, z + 6.2, 4));
  // These closed fins and rectangular handle sit flush on the cartridge.
  for (let i = 0; i < 4; i++) out.push(L(k.planOutline(plan(x + 5, y + 5 + i * (depth - 17) / 4, 21, 2), z + 6.2, P, 2), "mid"));
  out.push(S(plan(x + 7, y + depth - 7, 17, 4), z + 6, 3, { tone: "mid" }));
  out.push(S(plan(x + 5, y + 4, 4, 3), z + 6, 0.4, { tone: "hi", className: "pod-lamp" }));
  return group(`pod-${worker}-${index}`, out.join(""), `class="pod"`);
}

function worker(x, name) {
  const casing = plan(x, -68, 92, 104, 4);
  const surge = [S(plan(x + 5, -104, 82, 29, 2), 54, 2, { tone: "mid" })];
  for (const px of [x + 8, x + 77]) surge.unshift(S(plan(px, -85, 7, 7), 0, 54, { tone: "mid" }));
  surge.push(pod(x + 30, 2, name, -102, 25, 56));
  const out = [group(`surge-${name}`, surge.join("")), S(casing, 0, 8, { tone: "mid" })];
  out.push(S(k.insetPlan(casing, 2), 8, 30, { tone: "mid", inner: L(k.sideSeam(k.insetPlan(casing, 2), 23, P, 4)) }));
  out.push(S(k.insetPlan(casing, 1), 38, 3, { tone: "hi" }), bolts(casing, 41.2, 6));
  for (let i = 0; i < 2; i++) {
    out.push(S(plan(x + 7 + i * 43, -57, 4, 89), 41, 1, { tone: "mid" }));
    out.push(pod(x + 8 + i * 43, i, name));
  }
  for (let i = 0; i < 8; i++) out.push(L(k.planOutline(plan(x + 12 + i * 8, 23, 3, 9), 41.2, P, 2), "lo"));
  const back = [];
  for (const px of [x + 1, x + 85]) {
    back.push(S(plan(px, -66, 6, 6), 41, 33, { tone: "mid" }));
  }
  back.push(S(plan(x + 1, -66, 90, 6), 74, 4, { tone: "mid" }));
  const head = S(plan(x + 29, -67, 23, 8), 78, 5, { tone: "hi", lit: true });
  // Draw far gantry before cartridges, so the front handles stay visible.
  out.splice(4, 0, back.join(""), group(`scanner-${name}`, head));
  return group(name, out.join(""));
}

function transport(chapter) {
  const out = [];
  for (const y of [64, 89]) {
    out.push(S(plan(-180, y, 372, 5), 0, 7, { tone: "mid" }));
    out.push(L(k.planOutline(plan(-175, y + 1.7, 362, 1), 7.1, P, 2), "mid"));
  }
  for (const x of [-185, 192]) out.push(S(plan(x, 61, 5, 36), 0, 11, { tone: "mid" }));
  const shuttle = [S(plan(-170, 62, 43, 34, 2), 7, 4, { tone: "mid" }), S(plan(-165, 67, 33, 24, 2), 11, 2, { tone: "hi", lit: true })];
  shuttle.push(bolts(plan(-170, 62, 43, 34), 11.1, 3));
  out.push(group("shuttle", shuttle.join("")));
  if (chapter >= 3) {
    // A sliding coupling sits on a supported spur in front of Worker A.
    out.push(S(plan(-38, 39, 58, 19), 0, 4, { tone: "mid" }));
    out.push(S(plan(-35, 42, 18, 13), 4, 9, { tone: "mid" }));
    out.push(group("link", S(plan(-17, 42, 24, 13), 4, 9, { tone: "hi", lit: true })));
  }
  if (chapter === 4) {
    const gate = [];
    for (const y of [58, 96]) gate.push(S(plan(-70, y, 8, 8), 0, 70, { tone: "mid" }));
    gate.push(S(plan(-70, 58, 8, 46), 70, 4, { tone: "mid" }));
    gate.push(group("barrier", S(plan(-70, 66, 8, 30), 7, 29, { tone: "hi", lit: true })));
    out.push(group("gate", gate.join("")));
  }
  return group("rail", out.join(""));
}

export function machine(chapter) {
  const many = chapter >= 3;
  const body = [bench(), consoleUnit(chapter), worker(many ? -43 : 30, "a")];
  if (many) body.push(worker(85, "b"));
  body.push(transport(chapter));
  const svg = k.figureSvg({ width: WIDTH, height: HEIGHT, label: "HugoFleet 部署工作台。左側控制台提交操作卡，中間滑軌運送請求，右側工作站的卡匣呈現工作負載；上方巡檢滑塊代表 Operator。這是流程示意，不是即時 Kubernetes 狀態。", body });
  return { svg, solids: (svg.match(/class="iso-solid/g) || []).length };
}

export { ISO_CSS } from "./vendor/anatomy/iso-kit.mjs";
