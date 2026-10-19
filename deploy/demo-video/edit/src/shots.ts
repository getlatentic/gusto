// A shot aims the camera while one caption is on screen: where to look, as a fraction of the frame, and how close.
// The first caption that matches decides the shot. A caption no shot matches is shown at full frame.
export type Shot = { match: RegExp; at: [number, number]; zoom: number; caption?: string; top?: boolean; speed?: number; lapse?: number; maxZoom?: number; fixed?: boolean };

// A caption goes at the top when the shot looks at the lower half of the frame, so that it never covers what the shot is about,
// unless the shot says where.
export const captionAtTop = (shot?: Shot, at?: [number, number]): boolean => shot?.top ?? ((at ?? shot?.at)?.[1] ?? 0.5) > 0.55;

export const shots: Shot[] = [
  { match: /Choose the agent/, at: [0.32, 0.6], zoom: 1.35, top: false, caption: "Choose the agent to work with. Each gets the same workspace and controls." },
  { match: /Start it/, at: [0.3, 0.45], zoom: 1.7, caption: "Start it. The agent gets an isolated workspace of its own." },
  { match: /Only the models/, at: [0.5, 0.9], zoom: 1.5, speed: 1.5, caption: "Only the models your organization allows. The key never enters the sandbox." },
  { match: /works inside its sandbox/, at: [0.4, 0.42], zoom: 1.5, fixed: true, lapse: 5, caption: "It works inside its sandbox. The model key stays outside it." },
  { match: /Every step/, at: [0.4, 0.4], zoom: 1.35, caption: "Every step is recorded, down to the files it touched." },
  { match: /Preview what it built/, at: [0.5, 0.5], zoom: 1, top: true, speed: 3, caption: "Preview what it built, shared only with your organization." },
  { match: /loan calculator/i, at: [0.5, 0.45], zoom: 1.2, speed: 1.8, caption: "Running in the agent's workspace." },
  { match: /admin console/, at: [0.22, 0.22], zoom: 1.8, top: false, caption: "The admin console: what was spent, by whom, on which model." },
  { match: /Which models/, at: [0.58, 0.38], zoom: 1.35, speed: 1.3, caption: "Which models are approved, what each costs, and who may use which." },
  { match: /record of what happened/, at: [0.6, 0.62], zoom: 1.3, speed: 1.3, caption: "A record of what happened, who did it and when." },
  { match: /People/, at: [0.55, 0.3], zoom: 1.4, speed: 1.3, caption: "People and their roles." },
];
