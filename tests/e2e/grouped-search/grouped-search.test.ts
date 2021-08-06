/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import * as pg from '../helpers/page';
import * as search from '../helpers/search.page';

test('package: http', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?q=http',
    prepare: search.prepare,
  });
});

test('package: no results', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?q=zhttpz',
    prepare: search.prepare,
  });
});

test('identifier: context', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?m=identifiers&q=context',
    prepare: search.prepare,
  });
});

test('identifier: no results', async () => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, {
    path: '/search?m=identifiers&q=zhttpz',
    prepare: search.prepare,
  });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
