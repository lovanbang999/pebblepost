import { test, expect } from '@playwright/test'

test.describe('PebblePost Web Mode E2E Smoke Tests', () => {
  test('loads web studio, initializes UI, and displays primary workspace components', async ({ page }) => {
    // Navigate with token in query param
    await page.goto('/?token=smoke-token-test-123')

    // Expect page title
    await expect(page).toHaveTitle(/PebblePost Studio/)

    // Expect root container to be visible
    const root = page.locator('#root')
    await expect(root).toBeVisible()

    // Title bar exists and contains PebblePost branding
    const titleBar = page.locator('header')
    await expect(titleBar).toBeVisible()

    // Activity bar and sidebar view are present
    const activityBar = page.locator('nav')
    await expect(activityBar).toBeVisible()

    // Verify primary action buttons exist in sidebar
    const newRequestBtn = page.getByRole('button', { name: /new request/i })
    await expect(newRequestBtn).toBeVisible()

    // Click New Request to verify interactive functionality
    await newRequestBtn.click()
    const inlineInput = page.locator('input[placeholder*="request" i]')
    await expect(inlineInput).toBeVisible()

    // Press Escape to cancel inline creation
    await inlineInput.press('Escape')
    await expect(inlineInput).not.toBeVisible()
  })
})
