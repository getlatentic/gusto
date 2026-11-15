# web

The console's pages as a React app: shadcn components on Base UI, Tailwind v4, built by Vite into `internal/web/spa/dist`, which
the gusto binary embeds.

- The server serves the app at each page's own address (`/admin`, `/admin/usage`, `/admin/people`, …), behind the console's sign-in.
- A page reads its data at `/app/api` followed by its own address: `/admin/usage?state=failed` reads `/app/api/admin/usage?state=failed`.
- The server fills `<!--gusto-head-->` in `index.html` with gusto's tokens, faces and theme script, so a person's theme and accent
  apply here as on every other page. `src/index.css` maps shadcn's variables to those tokens.
- A page the app does not draw yet is a link to the server's own page (`src/components/app-link.tsx` lists the app's pages).

## Work on it

```bash
cd web
npm ci
npm run dev        # http://localhost:5173/app/admin, against a gusto server on :8700
npm test           # vitest, the pages drawn from fixed data
npm run lint
npm run build      # writes internal/web/spa/dist; commit it with the source
```

Add a shadcn component with `npx shadcn@latest add <name>`; `components.json` names the Base UI style.

## Files

```
src/main.tsx                the routes
src/components/ui/          shadcn's components, measured against Kylon where they differ
src/components/             the console's shell, the page bar, and the controls the pages share
src/pages/admin/            one file a page, with its test beside it
src/lib/api.ts              reading a page's data
src/lib/format.ts           money, counts and dates, written as the server's templates write them
src/lib/types.ts            the data each page reads
```
