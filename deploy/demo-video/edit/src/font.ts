import { continueRender, delayRender, staticFile } from "remotion";
import { useEffect, useState } from "react";

// Sora, the face the site uses, from the file the site carries.
export function useFont(): void {
  const [handle] = useState(() => delayRender("Sora"));
  useEffect(() => {
    const face = new FontFace("Sora", `url(${staticFile("sora.woff2")})`, { weight: "400 700" });
    face.load().then((loaded) => {
      (document.fonts as unknown as Set<FontFace>).add(loaded);
      continueRender(handle);
    });
  }, [handle]);
}
