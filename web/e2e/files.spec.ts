import type { Page } from "@playwright/test"

import {
  expect,
  roomWithAgent,
  test,
  unique,
  type Gusto,
  type Room,
} from "./fixtures.ts"

// roomWithFile is a room whose agent has written policy.md in the room's folder, which is one of the admin's projects.
async function roomWithFile(gusto: Gusto): Promise<Room> {
  const { room, agent } = await roomWithAgent(gusto)
  const turn = await gusto.say(room.path, `@${agent} write the policy`)
  await gusto.replied(room.path, turn)
  return room
}

const list = (page: Page) => page.getByTestId("files-list")

// name names the folder being made, and waits until it is made.
async function name(page: Page, folder: string) {
  const field = page.getByTestId("new-folder-name")
  await field.fill(folder)
  await field.press("Enter")
  await expect(field).toHaveCount(0)
}

test.describe("files", () => {
  test("a project made at the top is browsed into, and a folder made and filtered in it", async ({
    page,
  }) => {
    await page.goto("/files")
    await expect(page.getByTestId("page-title")).toHaveText("Files")
    const project = unique("Launch")
    await page.getByRole("button", { name: "New project" }).click()
    await name(page, project)
    await list(page).getByRole("link", { name: project }).click()

    await expect(page).toHaveURL(/\/files\?at=[^/&]+$/)
    await expect(page.getByTestId("page-title")).toHaveText(project)
    await expect(
      page.getByRole("navigation", { name: "Where this is" })
    ).toContainText("Files")
    await expect(list(page)).toContainText("This folder is empty.")

    const folder = unique("notes")
    await page.getByRole("button", { name: "New folder" }).click()
    await name(page, folder)
    const made = list(page).getByRole("link", { name: folder })
    await expect(made).toBeVisible()

    const filter = page.getByTestId("files-filter")
    await filter.fill(folder.toUpperCase())
    await expect(made).toBeVisible()
    await filter.fill("nothing-like-it")
    await expect(list(page)).toContainText("Nothing here matches.")
    await expect(made).toHaveCount(0)
    await filter.fill("")

    await made.click()
    await expect(page.getByTestId("page-title")).toHaveText(folder)
    await expect(list(page)).toContainText("This folder is empty.")
    await page.reload()
    await expect(page.getByTestId("page-title")).toHaveText(folder)

    await page
      .getByRole("navigation", { name: "Where this is" })
      .getByRole("link", { name: "Files" })
      .click()
    await expect(page).toHaveURL(/\/files$/)
    await expect(list(page).getByRole("link", { name: project })).toBeVisible()
  })

  test("a file opens, is changed and saved, and is still changed after a reload", async ({
    page,
    gusto,
  }) => {
    const room = await roomWithFile(gusto)
    await page.goto(`/files?at=${room.id}`)
    await list(page).getByRole("link", { name: "policy.md" }).click()

    await expect(page).toHaveURL(
      new RegExp(`/instances/${room.id}/file\\?path=policy\\.md&from=`)
    )
    await expect(page.getByTestId("page-title")).toHaveText("policy.md")
    const body = page.getByTestId("file-body")
    await expect(
      body.getByRole("heading", { name: "Spending policy" })
    ).toBeVisible()

    await page.getByRole("button", { name: "Edit" }).click()
    const text = page.getByRole("textbox", { name: "The text of policy.md" })
    await expect(text).toHaveValue(/# Spending policy/)
    await text.fill("# Travel policy\n\n- Trains, not planes\n")
    await page.getByRole("button", { name: "Save" }).click()
    await expect(
      body.getByRole("heading", { name: "Travel policy" })
    ).toBeVisible()

    await page.reload()
    await expect(
      page
        .getByTestId("file-body")
        .getByRole("heading", { name: "Travel policy" })
    ).toBeVisible()

    await page.getByTestId("file-back").click()
    await expect(page).toHaveURL(new RegExp(`/files\\?at=${room.id}$`))
  })

  test("Chat here starts a chat in the folder open", async ({
    page,
    gusto,
  }) => {
    const room = await roomWithFile(gusto)
    await page.goto(`/files?at=${room.id}`)
    await page.getByTestId("chat-here").click()
    await expect(page).toHaveURL(`/chat?at=${room.id}`)
  })

  test("a conversation's Files opens its folder, with the way back to it", async ({
    page,
    gusto,
  }) => {
    const room = await roomWithFile(gusto)
    await page.goto(`/instances/${room.id}/files?from=${room.path}`)
    await expect(page).toHaveURL(
      `/files?at=${room.id}&from=${encodeURIComponent(room.path)}`
    )
    await expect(
      list(page).getByRole("link", { name: "policy.md" })
    ).toBeVisible()
    await page.getByTestId("files-back").click()
    await expect(page).toHaveURL(room.path)
  })

  test.describe("at a phone's width", () => {
    test.use({ viewport: { width: 375, height: 812 }, isMobile: true })

    test("a folder and a file do not scroll sideways", async ({
      page,
      gusto,
    }) => {
      const room = await roomWithFile(gusto)
      const sideways = () =>
        page.evaluate(() => {
          const root = document.scrollingElement ?? document.documentElement
          return root.scrollWidth - root.clientWidth
        })
      await page.goto(`/files?at=${room.id}`)
      await expect(
        list(page).getByRole("link", { name: "policy.md" })
      ).toBeVisible()
      expect(await sideways()).toBe(0)
      await page.goto(`/instances/${room.id}/file?path=policy.md`)
      await expect(page.getByTestId("file-body")).toBeVisible()
      expect(await sideways()).toBe(0)
    })
  })
})
