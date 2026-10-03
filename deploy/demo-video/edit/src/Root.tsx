import { Composition, staticFile } from "remotion";
import { Demo, DemoProps, INTRO, OUTRO, TRIM } from "./Demo";

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
      return { durationInFrames: Math.round((INTRO + seconds + OUTRO) * FPS), props: { footageSeconds: seconds } };
    }}
  />
);

async function footageSeconds(): Promise<number> {
  const { getVideoMetadata } = await import("@remotion/media-utils");
  return (await getVideoMetadata(staticFile("footage.mp4"))).durationInSeconds;
}
