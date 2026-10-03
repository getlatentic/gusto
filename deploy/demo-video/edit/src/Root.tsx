import { Composition, staticFile } from "remotion";
import { Demo, DemoProps, INTRO, OUTRO, TRIM } from "./Demo";
import { lengthOf, takesOf } from "./camera";
import timeline from "../public/timeline.json";

const FPS = 30;

export const Root: React.FC = () => (
  <Composition
    id="Demo"
    component={Demo}
    width={1280}
    height={720}
    fps={FPS}
    durationInFrames={FPS * 90}
    defaultProps={{ footageSeconds: 80 } as DemoProps}
    calculateMetadata={async () => {
      const seconds = (await footageSeconds()) - TRIM;
      const shown = lengthOf(takesOf(timeline.map((m) => ({ ...m, t: Math.max(0, m.t - TRIM) })), seconds));
      return { durationInFrames: Math.round((INTRO + shown + OUTRO) * FPS), props: { footageSeconds: seconds } };
    }}
  />
);

async function footageSeconds(): Promise<number> {
  const { getVideoMetadata } = await import("@remotion/media-utils");
  return (await getVideoMetadata(staticFile("footage.mp4"))).durationInSeconds;
}
