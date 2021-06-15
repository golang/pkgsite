/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const puppeteer = require('puppeteer');
const NodeEnvironment = require('jest-environment-node');

const chromeURL = process.env.GO_DISCOVERY_E2E_CHROME_URL ?? 'ws://chrome:3000';

/**
 * PuppeteerEnvironment is a custom jest test environment. It extends the node
 * test environment to initialize global variables, connect puppeteer on
 * the host machine to the chromium instance.
 */
class PuppeteerEnvironment extends NodeEnvironment {
  constructor(config) {
    super(config);
    this.global.pageErrors = [];
  }

  async setup() {
    await super.setup();
    try {
      this.global.browser = await puppeteer.connect({
        browserWSEndpoint: chromeURL,
        defaultViewport: { height: 800, width: 1280 },
      });
    } catch (e) {
      console.error(e);
    }
  }

  async teardown() {
    await super.teardown();
    await this.global.browser.disconnect();
  }
}

module.exports = PuppeteerEnvironment;
