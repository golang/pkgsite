/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import * as pg from './helpers/page';
import * as search from './helpers/search.page';

test('no results', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?q=aoeuidhtns',
    prepare: search.prepare,
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
