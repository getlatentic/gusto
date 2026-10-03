// A shot aims the camera while one caption is on screen: where to look, as a fraction of the frame, and how close.
// The first caption that matches decides the shot. A caption no shot matches is shown at full frame.
export type Shot = { match: RegExp; at: [number, number]; zoom: number; caption?: string; top?: boolean };

// A caption goes at the top when the shot looks at the lower half of the frame, so that it never covers what the shot is about,
// unless the shot says where.
export const captionAtTop = (shot?: Shot): boolean => shot?.top ?? (shot ? shot.at[1] > 0.55 : false);

export const shots: Shot[] = [
  { match: /Choose the agent/, at: [0.32, 0.6], zoom: 1.35, top: false, caption: "Choose the agent to work with. Each gets the same workspace and controls." },
  { match: /Start it/, at: [0.3, 0.45], zoom: 1.7, caption: "Start it. The agent gets an isolated workspace of its own." },
  { match: /Only the models/, at: [0.5, 0.9], zoom: 1.5, caption: "Only the models your organisation allows. The key never enters the sandbox." },
  { match: /works inside its sandbox/, at: [0.82, 0.1], zoom: 1.8, caption: "It works inside its sandbox. The model key stays outside it." },
  { match: /Every step/, at: [0.4, 0.4], zoom: 1.35, caption: "Every step is recorded, down to the files it touched." },
  { match: /Preview what it built/, at: [0.5, 0.5], zoom: 1, top: true, caption: "Preview what it built, shared only with your organisation." },
  { match: /loan calculator/i, at: [0.5, 0.45], zoom: 1.2, caption: "Running in the agent's workspace." },
  { match: /admin console/, at: [0.2, 0.2], zoom: 1.8, top: true, caption: "The admin console: what was spent, by whom, on which model." },
  { match: /Which models/, at: [0.6, 0.35], zoom: 1.5, caption: "Which models are approved, what each costs, and who may use which." },
  { match: /record of what happened/, at: [0.5, 0.7], zoom: 1.3, caption: "A record of what happened, who did it and when." },
  { match: /People/, at: [0.4, 0.35], zoom: 1.5, caption: "People and their roles." },
];
