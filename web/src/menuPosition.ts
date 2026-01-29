export type Rect = {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
};

export type Size = { width: number; height: number };

export type Position = { left: number; top: number };

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max);
}

export function computePopupPosition(opts: {
  anchor: Rect;
  menu: Size;
  viewport: Size;
  margin?: number;
  offsetY?: number;
}): Position {
  const margin = Number.isFinite(opts.margin) ? (opts.margin as number) : 8;
  const offsetY = Number.isFinite(opts.offsetY) ? (opts.offsetY as number) : 8;

  const maxLeft = Math.max(margin, opts.viewport.width - opts.menu.width - margin);
  const maxTop = Math.max(margin, opts.viewport.height - opts.menu.height - margin);

  // Default: align menu right edge to anchor right edge, and place below.
  const idealLeft = opts.anchor.right - opts.menu.width;
  const idealTop = opts.anchor.bottom + offsetY;

  // If no space below, place above.
  const belowFits = idealTop + opts.menu.height + margin <= opts.viewport.height;
  const top = belowFits
    ? clamp(idealTop, margin, maxTop)
    : clamp(opts.anchor.top - offsetY - opts.menu.height, margin, maxTop);

  const left = clamp(idealLeft, margin, maxLeft);
  return { left, top };
}

