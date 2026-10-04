import React from "react";
import { AbsoluteFill, Freeze, Img, OffthreadVideo, Sequence, interpolate, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { brand } from "./brand";
import { Mark, marksOf, poseAt, takesOf, lengthOf, transformOf } from "./camera";
import { captionAtTop } from "./shots";
import { useFont } from "./font";
import timeline from "../public/timeline.json";

export const INTRO = 2.2;
export const OUTRO = 3.2;
// The page is blank while it loads, for the first moments of the recording.
export const TRIM = 1;
// A page fades out and the next fades in over this long, through the cream behind them: two pages laid over each other show both
// at once, and one page replaced by another between two frames is a jump.
export const FADE = 0.4;

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

const FadeIn: React.FC<{ frames: number; children: React.ReactNode }> = ({ frames, children }) => {
  const frame = useCurrentFrame();
  const opacity = frames ? interpolate(frame, [frames / 2, frames], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }) : 1;
  return <AbsoluteFill style={{ opacity }}>{children}</AbsoluteFill>;
};

const FadeOut: React.FC<{ frames: number; children: React.ReactNode }> = ({ frames, children }) => {
  const frame = useCurrentFrame();
  const opacity = interpolate(frame, [0, frames / 2], [1, 0], { extrapolateRight: "clamp" });
  return <AbsoluteFill style={{ opacity }}>{children}</AbsoluteFill>;
};

const Lapse: React.FC<{ speed: number; local: number; length: number }> = ({ speed, local, length }) => {
  const opacity = interpolate(local, [0, 0.3, length - 0.3, length], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={{ position: "absolute", bottom: 22, left: 28, opacity, padding: "7px 14px", borderRadius: 999, background: brand.orange, color: brand.ink, font: `700 18px/1 ${brand.font}` }}>
      Time-lapse ×{speed}
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
  const takes = takesOf(marksOf(timeline as Mark[], TRIM), footageSeconds);
  const time = now - INTRO;
  const take = takes.find((t) => time >= t.start && time < t.start + t.length);
  const local = take ? time - take.start : 0;
  const pose = poseAt(take, local);
  return (
    <AbsoluteFill style={{ background: brand.cream }}>
      <AbsoluteFill style={{ overflow: "hidden" }}>
        <AbsoluteFill style={transformOf(pose)}>
          {takes.map((t, i) => {
            const at = Math.round((INTRO + t.start) * fps);
            const length = Math.max(1, Math.round(t.length * fps));
            const dissolve = Math.round(FADE * fps);
            const last = Math.round((t.to + TRIM) * fps) - 1;
            return (
              <React.Fragment key={t.from}>
                <Sequence from={at} durationInFrames={length}>
                  <FadeIn frames={i === 0 ? 0 : dissolve}>
                    <OffthreadVideo src={staticFile("footage.mp4")} startFrom={Math.round((t.from + TRIM) * fps)} playbackRate={t.speed} style={{ width: "100%", height: "100%" }} />
                  </FadeIn>
                </Sequence>
                {i < takes.length - 1 ? (
                  <Sequence from={at + length} durationInFrames={dissolve}>
                    <FadeOut frames={dissolve}>
                      <Freeze frame={0}>
                        <OffthreadVideo src={staticFile("footage.mp4")} startFrom={last} style={{ width: "100%", height: "100%" }} />
                      </Freeze>
                    </FadeOut>
                  </Sequence>
                ) : null}
              </React.Fragment>
            );
          })}
        </AbsoluteFill>
        {take?.shot?.lapse ? <Lapse speed={take.shot.lapse} local={local} length={take.length} /> : null}
        {take?.caption ? <Caption text={take.caption} local={local} length={take.length} top={captionAtTop(take.shot, take.aim?.at)} /> : null}
      </AbsoluteFill>
      <Sequence durationInFrames={Math.round(INTRO * fps)}>
        <Card local={now} length={INTRO}>
          <Img src={staticFile("mark.png")} style={{ width: 64 }} />
          <div style={{ font: `700 56px/1.1 ${brand.font}`, letterSpacing: "-0.02em" }}>Latentic Agent Cloud</div>
          <div style={{ font: `500 28px/1.3 ${brand.font}`, color: brand.muted }}>Every agent, in one workspace, under one set of controls</div>
        </Card>
      </Sequence>
      <Sequence from={Math.round((INTRO + lengthOf(takes)) * fps)} durationInFrames={Math.round(OUTRO * fps)}>
        <Card local={now - INTRO - lengthOf(takes)} length={OUTRO}>
          <Img src={staticFile("mark.png")} style={{ width: 64 }} />
          <div style={{ font: `700 52px/1.15 ${brand.font}`, letterSpacing: "-0.02em", maxWidth: 900 }}>Explore a live workspace</div>
          <div style={{ padding: "16px 34px", borderRadius: 12, background: brand.orange, font: `700 30px/1 ${brand.font}` }}>agentcloud.getlatentic.com</div>
          <div style={{ font: `500 24px/1.3 ${brand.font}`, color: brand.muted }}>No sign-up. Deleted when it ends.</div>
        </Card>
      </Sequence>
    </AbsoluteFill>
  );
};
