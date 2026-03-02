import { expect, test, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";

const APP_URL = process.env.APP_URL ?? "http://localhost:5173";

const assertCreateSucceeded = async (page: Page) => {
  const infoBox = page.locator("p.mt-3.rounded-md.border.border-sky-200").first();
  await expect(infoBox).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(async () => (await infoBox.textContent()) ?? "", { timeout: 60_000 })
    .toMatch(/project created|项目创建成功|创建成功/i);
};

test("项目管理面板可完成 local_path 与 local_new 创建流程", async ({ page }) => {
  const runID = Date.now();
  const localPathName = `e2e-local-path-${runID}`;
  const localPathRepo = `D:/project/ai-workflow/.runtime/${localPathName}`;
  const localNewName = `e2e-local-new-${runID}`;
  const expectedLocalNewSlug = localNewName.toLowerCase();

  await page.goto(APP_URL, { waitUntil: "networkidle" });
  await expect(page.getByRole("heading", { name: "AI Workflow Workbench" })).toBeVisible();

  await page.selectOption("#create-source-type", "local_path");
  await page.fill("#create-project-name", localPathName);
  await page.fill("#create-repo-path", localPathRepo);
  await page.getByRole("button", { name: "创建项目" }).click();

  await expect(page.getByText("请求 ID:")).toBeVisible({ timeout: 15_000 });
  await assertCreateSucceeded(page);
  await expect
    .poll(async () => {
      return page
        .locator("#project-select option")
        .filter({ hasText: localPathName })
        .count();
    }, { timeout: 60_000 })
    .toBeGreaterThan(0);

  await page.selectOption("#create-source-type", "local_new");
  await page.fill("#create-project-name", localNewName);
  await page.getByRole("button", { name: "创建项目" }).click();

  await assertCreateSucceeded(page);
  await expect
    .poll(async () => {
      return page
        .locator("#project-select option")
        .filter({ hasText: localNewName })
        .count();
    }, { timeout: 60_000 })
    .toBeGreaterThan(0);

  await expect
    .poll(async () => {
      const hint = await page.locator("p.mt-2.text-xs.text-slate-500").textContent();
      return hint ?? "";
    }, { timeout: 60_000 })
    .toContain(expectedLocalNewSlug);

  const screenshotPath = join(".runtime", "playwright", `project-admin-${runID}.png`);
  mkdirSync(dirname(screenshotPath), { recursive: true });
  await page.screenshot({ path: screenshotPath, fullPage: true });
});
