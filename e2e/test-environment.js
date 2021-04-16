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
  // PORT default value should match ./global-setup.ts.
  PORT = 3000,
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
    this.global.newPage = async () => {
      const page = await this.global.browser.newPage();
      if (AUTHORIZATION) {
        await page.setRequestInterception(true);
        page.on('request', r => {
          const url = new URL(r.url());
          let headers = r.headers();
          if (url.origin.endsWith('pkg.go.dev')) {
            headers = { ...r.headers(), Authorization: `Bearer ${AUTHORIZATION}` };
          }
          r.continue({ headers });
        });
      }
      return page;
    };
  }

  async setup() {
    await super.setup();
    this.global.browser = await puppeteer.connect({
      browserWSEndpoint: `ws://localhost:${PORT}`,
      defaultViewport: { height: 800, width: 1280 },
    });
  }

  async teardown() {
    await super.teardown();
    await this.global.browser.disconnect();
  }
}

module.exports = PuppeteerEnvironment;
