import { Easing, interpolate } from "remotion";
import { Shot, shots } from "./shots";

// rect is where the step is on the page, as fractions of its size, when the recorder could find it.
export type Mark = { t: number; text: string; cut?: number; rect?: { x: number; y: number; w: number; h: number } };

// A take is what the recording showed while one caption was up: the stretch of footage it covers, how fast it plays, where it
// begins in the finished video and how long it lasts there.
export type Take = { from: number; to: number; speed: number; start: number; length: number; shot?: Shot; caption?: string; aim?: { at: [number, number]; zoom: number } };

// The recorded marks with the blank lead-in cut off.
export const marksOf = (recorded: Mark[], trim: number): Mark[] =>
  recorded.map((m) => ({ ...m, t: Math.max(0, m.t - trim), ...(m.cut === undefined ? {} : { cut: Math.max(0, m.cut - trim) }) }));

export function takesOf(marks: Mark[], total: number): Take[] {
  let start = 0;
  const takes: Take[] = [];
  marks.forEach((m, i) => {
    const next = marks[i + 1];
    const to = next ? next.cut ?? next.t : total;
    if (to - m.t <= 0.05) return;
    const shot = shots.find((s) => s.match.test(m.text));
    const speed = shot?.speed ?? 1;
    const length = (to - m.t) / speed;
    takes.push({ from: m.t, to, speed, start, length, shot, caption: m.text ? shot?.caption ?? m.text : undefined, aim: aimOf(m.rect, shot) });
    start += length;
  });
  return takes;
}

// The camera looks at where the recorder found the step, close enough for it to fill about three fifths of the frame, and no closer
// than the shot allows. Without a measurement it looks where the shot says.
const aimOf = (rect: Mark["rect"], shot?: Shot): Take["aim"] => {
  const fallback = shot ? { at: shot.at, zoom: shot.zoom } : undefined;
  if (shot?.fixed) return fallback;
  if (!rect || (rect.w < 0.08 && rect.h < 0.06)) return fallback;
  const zoom = Math.min(shot?.maxZoom ?? 1.8, 0.6 / Math.max(rect.w, rect.h));
  if (zoom < 1.15) return undefined;
  return { at: [rect.x + rect.w / 2, rect.y + rect.h / 2], zoom };
};

export const lengthOf = (takes: Take[]): number => takes.reduce((sum, t) => sum + t.length, 0);

const ease = Easing.bezier(0.45, 0, 0.2, 1);

// Every take opens on the whole page, so the viewer sees where they are, then the camera moves in, stays, and moves back out before
// the next take begins, so a change of page is always a cut between two whole views. A take too short for all of that stays wide.
const LOOK = 0.9;
const MOVE = 0.7;
const BACK = 0.6;
const HOLD = 0.6;

// The camera zooms about one fixed point, so the picture grows from where it is and never slides: the point is chosen so that the
// zoomed view ends centerd on what the take looks at, kept inside the page.
type Pose = { zoom: number; origin: [number, number] };
const wide: Pose = { zoom: 1, origin: [0.5, 0.5] };

const originFor = (at: [number, number], zoom: number): [number, number] => {
  const half = 0.5 / zoom;
  const center = (v: number) => Math.min(1 - half, Math.max(half, v));
  const o = (v: number) => (center(v) - half) / (1 - 1 / zoom);
  return [o(at[0]), o(at[1])];
};

export function poseAt(take: Take | undefined, local: number): Pose {
  if (!take?.aim || take.aim.zoom <= 1) return wide;
  if (take.length < LOOK + MOVE + HOLD + BACK) return wide;
  const into = interpolate(local, [LOOK, LOOK + MOVE], [0, 1], { easing: ease, extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const out = interpolate(local, [take.length - BACK, take.length], [1, 0], { easing: ease, extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const k = Math.min(into, out);
  return { zoom: 1 + (take.aim.zoom - 1) * k, origin: originFor(take.aim.at, take.aim.zoom) };
}

export const transformOf = (p: Pose): { transform: string; transformOrigin: string } => ({
  transform: `scale(${p.zoom})`,
  transformOrigin: `${p.origin[0] * 100}% ${p.origin[1] * 100}%`,
});
