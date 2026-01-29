import test from "node:test";
import assert from "node:assert/strict";
import { computePopupPosition } from "../src/menuPosition.ts";

test("computePopupPosition clamps to viewport and prefers below", () => {
  const pos = computePopupPosition({
    anchor: { left: 900, top: 100, right: 940, bottom: 120, width: 40, height: 20 },
    menu: { width: 180, height: 200 },
    viewport: { width: 960, height: 600 },
    margin: 8,
    offsetY: 8,
  });
  // right-aligned: 940 - 180 = 760; should fit
  assert.equal(pos.left, 760);
  // below: 120 + 8 = 128; should fit
  assert.equal(pos.top, 128);
});

test("computePopupPosition flips above when below would overflow", () => {
  const pos = computePopupPosition({
    anchor: { left: 500, top: 520, right: 540, bottom: 540, width: 40, height: 20 },
    menu: { width: 220, height: 160 },
    viewport: { width: 800, height: 600 },
    margin: 8,
    offsetY: 8,
  });
  // above: 520 - 8 - 160 = 352
  assert.equal(pos.top, 352);
});

test("computePopupPosition clamps left when anchor near left edge", () => {
  const pos = computePopupPosition({
    anchor: { left: 10, top: 100, right: 30, bottom: 120, width: 20, height: 20 },
    menu: { width: 240, height: 120 },
    viewport: { width: 320, height: 400 },
    margin: 8,
    offsetY: 8,
  });
  assert.equal(pos.left, 8);
});
