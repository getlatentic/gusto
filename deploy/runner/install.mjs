// Installs the npm packages the runner image carries, each at the version and bytes pins.json names.
//
//   node install.mjs <pins.json> <record> <member>...
//
// A member is name=package (an agent) or a bare package (an MCP server). A member may carry @version, and it must be the
// pinned one. Each tarball is fetched with npm pack, its sha512 compared with the pin before anything is installed, and
// its line (name=package@version, or package@version) appended to the record file the image keeps at /etc/gusto.
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { appendFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const [pinsPath, recordPath, ...members] = process.argv.slice(2);
if (!pinsPath || !recordPath) fail("usage: install.mjs <pins.json> <record> <member>...");
const pins = JSON.parse(readFileSync(pinsPath, "utf8")).packages;
mkdirSync(join(recordPath, ".."), { recursive: true });
appendFileSync(recordPath, "");
const stage = mkdtempSync(join(tmpdir(), "gusto-install-"));
// npm's cache is kept in the stage and removed with it: left in the image it is hundreds of megabytes of tarballs nothing reads again.
const npm = { ...process.env, npm_config_cache: join(stage, "npm-cache") };

for (const member of members) {
  const { name, pkg, version } = parse(member);
  const pin = pins[pkg];
  if (!pin) fail(`${pkg} is not in pins.json: run pkg/harness/pin.sh ${pkg}@<version>`);
  if (version && version !== pin.Version && version !== pin.version)
    fail(`${member} asks for ${version}, and pins.json pins ${pin.version}: move the pin or drop the version`);
  const packed = JSON.parse(execFileSync("npm", ["pack", `${pkg}@${pin.version}`, "--pack-destination", stage, "--json"], { encoding: "utf8", env: npm }));
  const file = join(stage, packed[0].filename);
  const sum = "sha512-" + createHash("sha512").update(readFileSync(file)).digest("base64");
  if (sum !== pin.integrity) fail(`${pkg}@${pin.version}: the tarball's sha512 is not the pinned one\n  got  ${sum}\n  want ${pin.integrity}`);
  execFileSync("npm", ["install", "-g", "--no-fund", "--no-audit", file], { stdio: "inherit", env: npm });
  appendFileSync(recordPath, `${name ? name + "=" : ""}${pkg}@${pin.version}\n`);
  console.log(`${name || pkg}: ${pkg}@${pin.version} verified and installed`);
}
rmSync(stage, { recursive: true, force: true });

// parse splits name=package@version into its parts. A scoped package starts with @, so only an @ after the first
// character names a version.
function parse(member) {
  const [name, rest] = member.includes("=") ? member.split("=", 2) : ["", member];
  const at = rest.indexOf("@", 1);
  if (!rest || (member.includes("=") && !name)) fail(`${member}: say name=package, or a package`);
  return at < 0 ? { name, pkg: rest, version: "" } : { name, pkg: rest.slice(0, at), version: rest.slice(at + 1) };
}

function fail(message) {
  console.error(message);
  process.exit(1);
}
