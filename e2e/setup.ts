import fs from 'fs';
import os from 'os';
import path from 'path';
import { toMatchImageSnapshot } from 'jest-image-snapshot';
import puppeteer, { Browser } from 'puppeteer';

declare const global: NodeJS.Global & typeof globalThis & { browser: Browser };

expect.extend({ toMatchImageSnapshot });

beforeAll(async () => {
  const DIR = path.join(os.tmpdir(), 'jest_puppeteer_global_setup');
  const wsEndpoint = fs.readFileSync(path.join(DIR, 'wsEndpoint'), 'utf8');
  if (!wsEndpoint) {
    throw new Error('wsEndpoint not found');
  }

  global.browser = await puppeteer.connect({
    browserWSEndpoint: wsEndpoint,
    defaultViewport: { height: 800, width: 1280 },
  });
});

afterAll(async () => {
  global.browser.disconnect();
});
