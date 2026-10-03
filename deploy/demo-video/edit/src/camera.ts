import { Easing, interpolate } from "remotion";
import { Shot, shots } from "./shots";

export type Mark = { t: number; text: string };

// A take is what the recording showed while one caption was up: the stretch of footage it covers, how fast it plays, where it
// begins in the finished video and how long it lasts there.
export type Take = { from: number; to: number; speed: number; start: number; length: number; shot?: Shot; caption?: string };

export function takesOf(marks: Mark[], total: number): Take[] {
  let start = 0;
  const takes: Take[] = [];
  marks.forEach((m, i) => {
    const to = marks[i + 1]?.t ?? total;
    if (to - m.t <= 0.05) return;
    const shot = shots.find((s) => s.match.test(m.text));
    const speed = shot?.speed ?? 1;
    const length = (to - m.t) / speed;
    takes.push({ from: m.t, to, speed, start, length, shot, caption: m.text ? shot?.caption ?? m.text : undefined });
    start += length;
  });
  return takes;
}

export const lengthOf = (takes: Take[]): number => takes.reduce((sum, t) => sum + t.length, 0);

const ease = Easing.bezier(0.45, 0, 0.2, 1);

// Every take opens on the whole page, so the viewer sees where they are, then the camera moves in, stays, and moves back out before
// the next take begins, so a change of page is always a cut between two whole views. A take too short for all of that stays wide.
const LOOK = 0.9;
const MOVE = 0.7;
const BACK = 0.6;
const HOLD = 0.6;

type Pose = { zoom: number; x: number; y: number };
const wide: Pose = { zoom: 1, x: 0.5, y: 0.5 };

export function poseAt(take: Take | undefined, local: number): Pose {
  if (!take?.shot || take.shot.zoom <= 1) return wide;
  if (take.length < LOOK + MOVE + HOLD + BACK) return wide;
  const here: Pose = { zoom: take.shot.zoom, x: take.shot.at[0], y: take.shot.at[1] };
  const into = interpolate(local, [LOOK, LOOK + MOVE], [0, 1], { easing: ease, extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const out = interpolate(local, [take.length - BACK, take.length], [1, 0], { easing: ease, extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const k = Math.min(into, out);
  return { zoom: wide.zoom + (here.zoom - wide.zoom) * k, x: wide.x + (here.x - wide.x) * k, y: wide.y + (here.y - wide.y) * k };
}

// A transform that puts the pose's point at the middle of the frame without showing beyond the frame's edges.
export function transformOf(p: Pose): string {
  const clamp = (v: number, half: number) => Math.min(1 - half, Math.max(half, v));
  const half = 0.5 / p.zoom;
  const x = clamp(p.x, half);
  const y = clamp(p.y, half);
  return `scale(${p.zoom}) translate(${(0.5 - x) * 100}%, ${(0.5 - y) * 100}%)`;
}
