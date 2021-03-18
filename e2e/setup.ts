import { toMatchImageSnapshot } from 'jest-image-snapshot';
import puppeteer, { Browser } from 'puppeteer';

declare const global: NodeJS.Global & typeof globalThis & { browser: Browser };

expect.extend({ toMatchImageSnapshot });

beforeAll(async () => {
  global.browser = await puppeteer.launch({
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
    defaultViewport: { height: 800, width: 1280 },
  });
});

afterAll(async () => await global.browser.close());
