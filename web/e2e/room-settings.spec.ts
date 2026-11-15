import {
  expect,
  sideRow,
  test,
  unique,
  type Gusto,
  type Room,
} from "./fixtures.ts"
import { runner } from "./values.mjs"

async function roomWithRunner(gusto: Gusto, name: string): Promise<Room> {
  const room = await gusto.makeRoom(unique(name))
  await gusto.addToRoom(room, { people: [runner.email] })
  return room
}

test("a muted room counts nothing unread in the side list, and an archived room gives way to its banner until it is unarchived", async ({
  page,
  gusto,
  runnerGusto,
}) => {
  const muted = await roomWithRunner(gusto, "Muted ")
  const loud = await roomWithRunner(gusto, "Loud ")

  await page.goto(muted.path)
  await page.getByTestId("room-menu").click()
  await expect(page.getByTestId("room-mute")).toHaveText(/Mute room/)
  await page.getByTestId("room-mute").click()
  await page.getByTestId("room-menu").click()
  await expect(page.getByTestId("room-mute")).toHaveText(/Unmute room/)
  await page.keyboard.press("Escape")

  // The runner speaks in both rooms; the side list counts it in the room that is not muted, and not in the muted one.
  await page.goto("/chat")
  await runnerGusto.say(muted.path, "anyone here?")
  await runnerGusto.say(loud.path, "anyone here?")
  await page.reload()
  const sideRooms = page.getByTestId("side-rooms")
  await expect(
    sideRow(sideRooms, loud.path).getByTestId("unread-count")
  ).toHaveText("1")
  await expect(sideRow(sideRooms, muted.path)).toBeVisible()
  await expect(
    sideRow(sideRooms, muted.path).getByTestId("unread-count")
  ).toHaveCount(0)

  await sideRow(sideRooms, muted.path).click()
  await expect(page.getByTestId("message-box")).toBeVisible()
  await page.getByTestId("room-menu").click()
  await page.getByTestId("room-archive").click()
  await expect(page.getByTestId("room-confirm")).toBeVisible()
  await page.getByTestId("room-confirm-action").click()

  await expect(page.getByTestId("archived")).toBeVisible()
  await expect(page.getByTestId("message-box")).toHaveCount(0)
  await expect(
    sideRow(page.getByTestId("side-archived"), muted.path)
  ).toBeVisible()
  await expect(sideRow(sideRooms, muted.path)).toHaveCount(0)

  await page.getByTestId("unarchive").click()
  await expect(page.getByTestId("message-box")).toBeVisible()
  await expect(page.getByTestId("archived")).toHaveCount(0)
  await expect(sideRow(sideRooms, muted.path)).toBeVisible()
})
