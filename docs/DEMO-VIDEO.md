# The demo video

The landing page at `https://<name>/` shows a video recorded from the live demo. It is
`internal/cloud/static/demo.mp4`, which the front door carries, so nothing else hosts it. An operator may show another by
setting `DEMO_VIDEO_URL` in `demo.env`, stored with `deploy/aws/openshell/put-demo-config.sh`: an https address of a file
(`.mp4` or `.webm`) or of a page that may be embedded (Loom, YouTube's `youtube-nocookie.com/embed/...`, Vimeo's player).

## Recording it again

`deploy/demo-video/record.mjs` drives the live demo as a person already in the platform and records it, with captions: choosing
an agent, an agent building a loan repayment calculator for John Smith of Future Acme Bank, its preview, the admin console
and the end page. The demo is started off camera. To have an end page to show, make and end a demo for the same person first,
and give its address as the second argument. It marks the waits it makes.
`encode.sh` plays those fast and makes `haas-demo.mp4` and `poster.jpg`, about a minute and a quarter in all. Copy them to
`internal/cloud/static/demo.mp4` and `demo-poster.jpg`.

```bash
cd deploy/demo-video
PLAYWRIGHT_CORE=<playwright-core> CHROME=<a chromium> node record.mjs https://<name> https://<an ended demo's address>
./encode.sh
```

A recording is a demo of its own, so a day's recordings count against the limits of one address and one company
(`HAAS_CLOUD_DEMO_PER_PERSON`, `HAAS_CLOUD_DEMO_PER_COMPANY`, `HAAS_CLOUD_DEMO_PER_DAY`). To record more than once in a day,
lift them on the host for the run only, with a systemd drop-in whose `EnvironmentFile` is in `/run`, and take it off after.
The bot check also stops a recording, since a script is what it is for: for the run, the same drop-in can hold
Cloudflare's published test keys (`HAAS_CLOUD_TURNSTILE_SITE=1x00000000000000000000AA`,
`HAAS_CLOUD_TURNSTILE_SECRET=1x0000000000000000000000000000000AA`), which pass every check. Take the drop-in off and
restart the front door at once afterwards.

Before recording: end every demo on the host, since only five run at once and a full host turns the recording away,
and stop the cells so that the next one starts from the release you are showing. The first demo on a new cell waits a few
minutes, which `encode.sh` shortens. The model decides what the agent writes, so look at the result before keeping it.

The script of what it shows, for a person to record instead:

## Script, about two minutes

Record it at 1280 × 720 on the live demo, in a window with nothing else open. Use a work address and a real company
name. Say what is on the screen, and nothing that is not.

1. **The page (10 s).** "haas gives each person on a team coding agents that work in a computer of their own." Click
   **Start a 30 minute demo**.
2. **Start (10 s).** Type a name, an address and a company. Press **Start my demo**. The wait page says a machine is
   starting. Cut to when it is ready: "your time starts when it is ready."
3. **An agent (40 s).** In the organisation, start an agent, pick a model and ask for a small real task, such as
   "add a health check to this service and a test for it". Show it working and the files it changed. Point at the
   countdown in the corner.
4. **The admin side (40 s).** Open the console. Show **Overview** (what it has spent), **Models** (the providers, what
   each model costs, limiting a person to some models) and **People**.
5. **The end (10 s).** "When the time is up, the organisation and everything in it is deleted." Show the end page
   and **Book a call**.

Keep the take that shows the whole wait. Do not speed it up: the wait is part of what a visitor will see.
