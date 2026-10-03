import { AbsoluteFill, OffthreadVideo, Sequence, interpolate, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { brand } from "./brand";
import { Take, poseAt, takesOf, transformOf } from "./camera";
import { captionAtTop } from "./shots";
import { useFont } from "./font";
import timeline from "../public/timeline.json";

export const INTRO = 2.2;
export const OUTRO = 3.2;
// The page is blank while it loads, for the first moments of the recording.
export const TRIM = 1;

export type DemoProps = { footageSeconds: number };

const Caption: React.FC<{ text: string; local: number; length: number; top: boolean }> = ({ text, local, length, top }) => {
  const opacity = interpolate(local, [0, 0.3, length - 0.3, length], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const lift = interpolate(local, [0, 0.3], [14, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={{ position: "absolute", left: 0, right: 0, ...(top ? { top: 24 } : { bottom: 22 }), display: "flex", justifyContent: "center", opacity, transform: `translateY(${lift}px)` }}>
      <div style={{ maxWidth: 880, padding: "8px 18px", borderRadius: 10, background: "rgba(23,23,23,.86)", color: "#fff", font: `600 21px/1.3 ${brand.font}`, textAlign: "center" }}>{text}</div>
    </div>
  );
};

const Card: React.FC<{ children: React.ReactNode; local: number; length: number }> = ({ children, local, length }) => {
  const opacity = interpolate(local, [0, 0.4, length - 0.4, length], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <AbsoluteFill style={{ background: brand.cream, alignItems: "center", justifyContent: "center", fontFamily: brand.font, color: brand.ink }}>
      <div style={{ opacity, textAlign: "center", display: "grid", gap: 22, justifyItems: "center" }}>{children}</div>
    </AbsoluteFill>
  );
};

export const Demo: React.FC<DemoProps> = ({ footageSeconds }) => {
  useFont();
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const now = frame / fps;
  const takes: Take[] = takesOf(timeline.map((m) => ({ ...m, t: Math.max(0, m.t - TRIM) })), footageSeconds);
  const time = now - INTRO;
  const pose = poseAt(takes, time);
  const take = takes.find((t) => time >= t.from && time < t.to);
  return (
    <AbsoluteFill style={{ background: brand.cream }}>
      <Sequence from={Math.round(INTRO * fps)} durationInFrames={Math.round(footageSeconds * fps)}>
        <AbsoluteFill style={{ overflow: "hidden" }}>
          <AbsoluteFill style={{ transform: transformOf(pose), transformOrigin: "50% 50%" }}>
            <OffthreadVideo src={staticFile("footage.mp4")} startFrom={Math.round(TRIM * fps)} style={{ width: "100%", height: "100%" }} />
          </AbsoluteFill>
          {take?.caption ? <Caption text={take.caption} local={time - take.from} length={take.to - take.from} top={captionAtTop(take.shot)} /> : null}
        </AbsoluteFill>
      </Sequence>
      <Sequence durationInFrames={Math.round(INTRO * fps)}>
        <Card local={now} length={INTRO}>
          <div style={{ font: `700 56px/1.1 ${brand.font}`, letterSpacing: "-0.02em" }}>Latentic Agent Cloud</div>
          <div style={{ font: `500 28px/1.3 ${brand.font}`, color: brand.muted }}>Every agent, in one workspace, under one set of controls</div>
        </Card>
      </Sequence>
      <Sequence from={Math.round((INTRO + footageSeconds) * fps)} durationInFrames={Math.round(OUTRO * fps)}>
        <Card local={now - INTRO - footageSeconds} length={OUTRO}>
          <div style={{ font: `700 52px/1.15 ${brand.font}`, letterSpacing: "-0.02em", maxWidth: 900 }}>Explore a live workspace</div>
          <div style={{ padding: "16px 34px", borderRadius: 12, background: brand.orange, font: `700 30px/1 ${brand.font}` }}>agentcloud.getlatentic.com</div>
          <div style={{ font: `500 24px/1.3 ${brand.font}`, color: brand.muted }}>No sign-up. Deleted when it ends.</div>
        </Card>
      </Sequence>
    </AbsoluteFill>
  );
};
