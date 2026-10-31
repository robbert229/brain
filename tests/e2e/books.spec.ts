import { expect, test, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';

const today = () => new Date().toISOString().slice(0, 10);
const controlURL = process.env.E2E_CONTROL_URL ?? 'http://127.0.0.1:4175';

test.beforeEach(async ({ request }) => {
  const response = await request.post(`${controlURL}/__test/role`, { data: { role: 'user' } });
  expect(response.status()).toBe(204);
});

async function openShelf(page: Page) {
  await page.goto('/books');
  await expect(page).toHaveURL(/\/books$/);
  await expect(page.getByRole('heading', { name: 'Books', exact: true })).toBeVisible();
}

test('a contributor creates and edits a book review with a cover in the browser', async ({ page }) => {
  await openShelf(page);
  const initialBookCount = await page.locator('.book-row').count();
  const createdTitle = `Browser Created Book ${randomUUID()}`;
  const editedTitle = `${createdTitle} Edited`;
  await page.getByRole('link', { name: 'Add a review' }).click();

  await expect(page.locator('#review-finished')).toHaveValue(today());
  await page.locator('#review-title').fill(createdTitle);
  await page.locator('#review-author').fill('Test Author');
  await page.locator('#review-body').fill('Created through a real browser session.');
  await page.locator('#review-cover-upload').setInputFiles({
    name: 'cover.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p0sAAAAASUVORK5CYII=', 'base64'),
  });
  await page.getByRole('button', { name: 'Save review' }).click();

  const row = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: createdTitle }) });
  await expect(row).toBeVisible();
  const cover = row.locator('img.book-cover');
  await expect(cover).toHaveAttribute('src', /\/books\/covers\/book-cover-[a-f0-9]{64}\.png$/);
  await expect.poll(() => cover.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
  await expect(row.locator('time')).toHaveAttribute('datetime', today());

  await row.getByRole('link', { name: 'Edit review' }).click();
  await expect(page.locator('#review-finished')).toHaveValue(today());
  await expect(page.locator('#review-cover')).toHaveValue(/book-cover-[a-f0-9]{64}\.png/);
  await page.locator('#review-title').fill(editedTitle);
  await page.locator('#review-body').fill('Edited through the same browser flow.');
  await page.locator('#review-cover-upload').setInputFiles({
    name: 'replacement.gif',
    mimeType: 'image/gif',
    buffer: Buffer.from('R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=', 'base64'),
  });
  await page.getByRole('button', { name: 'Save changes' }).click();

  await expect(page.locator('.book-row')).toHaveCount(initialBookCount + 1);
  const edited = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: editedTitle }) });
  await expect(edited).toBeVisible();
  await expect(edited.locator('img.book-cover')).toHaveAttribute('src', /\/books\/covers\/book-cover-[a-f0-9]{64}\.gif$/);
  await expect(edited.locator('.book-review')).toHaveText('Edited through the same browser flow.');
});

test('an undated existing book defaults Finished to today when editing', async ({ page }) => {
  await openShelf(page);
  const row = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: 'E2E Undated Book' }) });
  await row.getByRole('link', { name: 'Edit review' }).click();
  await expect(page.locator('#review-finished')).toHaveValue(today());
});

test('a viewer can read the shelf but cannot create or edit reviews', async ({ page, request }) => {
  const roleResponse = await request.post(`${controlURL}/__test/role`, { data: { role: 'viewer' } });
  expect(roleResponse.status()).toBe(204);

  await openShelf(page);
  await expect(page.getByRole('link', { name: 'Add a review' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Edit review' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Delete review' })).toHaveCount(0);

  const createResponse = await page.goto('/books/new');
  expect(createResponse?.status()).toBe(403);
  const editResponse = await page.goto('/books/edit?file=E2E%20Preserved%20Book.md');
  expect(editResponse?.status()).toBe(403);
  const deleteResponse = await page.goto('/books/delete?file=E2E%20Preserved%20Book.md');
  expect(deleteResponse?.status()).toBe(403);
});

test('deleting a review requires confirmation and cancel leaves it intact', async ({ page }) => {
  await openShelf(page);
  const title = `Browser Delete Book ${randomUUID()}`;
  await page.getByRole('link', { name: 'Add a review' }).click();
  await page.locator('#review-title').fill(title);
  await page.locator('#review-body').fill('This review will be deleted after confirmation.');
  await page.getByRole('button', { name: 'Save review' }).click();

  const row = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: title }) });
  await expect(row).toBeVisible();
  await row.getByRole('link', { name: 'Delete review' }).click();
  await expect(page.getByRole('heading', { name: 'Delete review?' })).toBeVisible();
  await expect(page.getByRole('heading', { name: title })).toBeVisible();
  await expect(page.getByText(/permanently deletes the Markdown review note/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Delete review' })).toBeVisible();

  await page.getByRole('link', { name: 'Cancel' }).click();
  await expect(row).toBeVisible();
  await row.getByRole('link', { name: 'Delete review' }).click();
  await page.getByRole('button', { name: 'Delete review' }).click();

  await expect(page).toHaveURL(/\/books$/);
  await expect(page.getByRole('heading', { name: title })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('heading', { name: title })).toHaveCount(0);
});

test('books without a usable cover show the accessible fallback cover', async ({ page }) => {
  await openShelf(page);
  const row = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: 'E2E Undated Book' }) });
  const fallback = row.locator('img.book-cover');
  await expect(fallback).toHaveAttribute('src', '/static/book-cover-placeholder.svg');
  await expect(fallback).toHaveAttribute('alt', 'No cover available for E2E Undated Book');
  await expect.poll(() => fallback.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
});

test('editing one field preserves the existing cover and other visible book metadata', async ({ page }) => {
  await openShelf(page);
  const row = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: 'E2E Preserved Book' }) });
  await row.getByRole('link', { name: 'Edit review' }).click();

  await expect(page.locator('#review-author')).toHaveValue('Preservation Author');
  await expect(page.locator('#review-rating')).toHaveValue('4/5');
  await expect(page.locator('#review-cover')).toHaveValue('fixture-cover.png');
  await expect(page.locator('#review-finished')).toHaveValue('2025-03-04');
  await expect(page.locator('#review-body')).toHaveValue('The original review remains visible.');
  await page.locator('#review-title').fill('E2E Preserved Book Updated');
  await page.getByRole('button', { name: 'Save changes' }).click();

  const updated = page.locator('.book-row').filter({ has: page.getByRole('heading', { name: 'E2E Preserved Book Updated' }) });
  await expect(updated.locator('.book-author')).toHaveText('Preservation Author');
  await expect(updated.locator('.book-status')).toHaveText('Finished');
  await expect(updated.locator('.book-rating')).toHaveAttribute('aria-label', '4/5');
  await expect(updated.locator('time')).toHaveAttribute('datetime', '2025-03-04');
  await expect(updated.locator('img.book-cover')).toHaveAttribute('src', '/books/covers/fixture-cover.png');
  await expect(updated.locator('.book-review')).toHaveText('The original review remains visible.');
});

test('an invalid uploaded cover preserves the draft and does not add a book', async ({ page }) => {
  await openShelf(page);
  const initialBookCount = await page.locator('.book-row').count();
  await page.getByRole('link', { name: 'Add a review' }).click();
  await page.locator('#review-title').fill('Invalid Cover Draft');
  await page.locator('#review-body').fill('This draft must not be saved.');
  await page.locator('#review-cover-upload').setInputFiles({
    name: 'not-an-image.png',
    mimeType: 'image/png',
    buffer: Buffer.from('not an image'),
  });
  await page.getByRole('button', { name: 'Save review' }).click();

  await expect(page.getByText('Use a valid JPG, PNG, GIF, or WebP image.')).toBeVisible();
  await expect(page.locator('#review-title')).toHaveValue('Invalid Cover Draft');
  await expect(page.locator('#review-body')).toHaveValue('This draft must not be saved.');
  await page.getByRole('link', { name: 'personal library' }).click();
  await expect(page.locator('.book-row')).toHaveCount(initialBookCount);
  await expect(page.getByRole('heading', { name: 'Invalid Cover Draft' })).toHaveCount(0);
});

test('search, status filters, and the brain breadcrumb work at desktop and mobile widths', async ({ page }) => {
  await openShelf(page);
  await expect(page.getByRole('link', { name: 'brain', exact: true })).toHaveAttribute('href', '/');

  await page.locator('#book-search').fill('undated');
  await expect(page.locator('.books-count')).toHaveText('1 book');
  await page.getByRole('button', { name: 'Reading' }).click();
  await expect(page.locator('.books-count')).toHaveText('0 books');
  await expect(page.locator('.books-no-results')).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('button', { name: 'All books' }).click();
  await page.locator('#book-search').fill('');
  await expect(page.locator('.book-row').first()).toBeVisible();
  await page.getByRole('link', { name: 'brain', exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
});
