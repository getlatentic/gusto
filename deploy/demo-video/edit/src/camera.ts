import { Easing, interpolate } from "remotion";
import { Shot, shots } from "./shots";

export type Mark = { t: number; text: string };
export type Take = { from: number; to: number; shot?: Shot; caption?: string };

// Each caption the recording said becomes a take that lasts until the next one.
export function takesOf(marks: Mark[], total: number): Take[] {
  return marks
    .filter((m, i) => m.text || i < marks.length)
    .map((m, i) => {
      const next = marks[i + 1]?.t ?? total;
      const shot = shots.find((s) => s.match.test(m.text));
      return { from: m.t, to: next, shot, caption: m.text ? shot?.caption ?? m.text : undefined };
    })
    .filter((t) => t.to - t.from > 0.05);
}

const SETTLE = 0.7;
const ease = Easing.bezier(0.45, 0, 0.2, 1);

type Pose = { zoom: number; x: number; y: number };
const wide: Pose = { zoom: 1, x: 0.5, y: 0.5 };
const poseOf = (t?: Take): Pose => (t?.shot ? { zoom: t.shot.zoom, x: t.shot.at[0], y: t.shot.at[1] } : wide);

// The camera moves into a take's pose over SETTLE seconds from its start, and stays until the next take begins.
export function poseAt(takes: Take[], time: number): Pose {
  const index = takes.findIndex((t) => time >= t.from && time < t.to);
  if (index < 0) return wide;
  const here = poseOf(takes[index]);
  const before = index > 0 ? poseOf(takes[index - 1]) : wide;
  const k = interpolate(time - takes[index].from, [0, SETTLE], [0, 1], { easing: ease, extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return { zoom: before.zoom + (here.zoom - before.zoom) * k, x: before.x + (here.x - before.x) * k, y: before.y + (here.y - before.y) * k };
}

// A transform that puts the pose's point at the middle of the frame without showing beyond the frame's edges.
export function transformOf(p: Pose): string {
  const clamp = (v: number, half: number) => Math.min(1 - half, Math.max(half, v));
  const half = 0.5 / p.zoom;
  const x = clamp(p.x, half);
  const y = clamp(p.y, half);
  return `scale(${p.zoom}) translate(${(0.5 - x) * 100}%, ${(0.5 - y) * 100}%)`;
}
