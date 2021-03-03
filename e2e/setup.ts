import { toMatchImageSnapshot } from 'jest-image-snapshot';
import puppeteer, { Browser, Page } from 'puppeteer';

declare const global: NodeJS.Global & typeof globalThis & { browser: Browser; page: Page };

expect.extend({ toMatchImageSnapshot });

beforeAll(async () => {
  global.browser = await puppeteer.launch({
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
    defaultViewport: { height: 800, width: 1280 },
  });
  global.page = await global.browser.newPage();
});

afterAll(async () => await global.browser.close());
