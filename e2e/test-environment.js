/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const puppeteer = require('puppeteer');
const NodeEnvironment = require('jest-environment-node');

const {
  AUTHORIZATION = null,
  BASE_URL = 'http://host.docker.internal:8080',
  // GO_DISCOVERY_E2E_TEST_PORT default value should match ./global-setup.ts.
  GO_DISCOVERY_E2E_TEST_PORT = 3000,
} = process.env;

/**
 * PuppeteerEnvironment is a custom jest test environment. It extends the node
 * test environment to initialize global variables, connect puppeteer on
 * the host machine to the chromium instance running in docker, and add
 * authorization to requests when the AUTHORIZATION env var is set.
 */
class PuppeteerEnvironment extends NodeEnvironment {
  constructor(config) {
    super(config);
    this.global.baseURL = BASE_URL;
    this.global.pageErrors = [];
    this.global.newPage = async () => {
      const page = await this.global.browser.newPage();
      if (AUTHORIZATION) {
        await page.setRequestInterception(true);
        page.on('request', r => {
          const url = new URL(r.url());
          let headers = r.headers();
          if (url.origin === BASE_URL) {
            headers = { ...r.headers(), Authorization: `Bearer ${AUTHORIZATION}` };
          }
          r.continue({ headers });
        });
      }
      page.on('pageerror', err => {
        this.global.pageErrors.push(err);
      });
      return page;
    };
  }

  async setup() {
    await super.setup();
    this.global.browser = await puppeteer.connect({
      browserWSEndpoint: `ws://localhost:${GO_DISCOVERY_E2E_TEST_PORT}`,
      defaultViewport: { height: 800, width: 1280 },
    });
  }

  async teardown() {
    await super.teardown();
    await this.global.browser.disconnect();
  }
}

module.exports = PuppeteerEnvironment;
