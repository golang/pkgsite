/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import * as pg from './helpers/page';
import * as search from './helpers/search.page';

export const testcases = test.each`
  name                       | path                                    | prepare
  ${'badge'}                 | ${'/badge'}                             | ${pg.prepare}
  ${'error'}                 | ${'/bad.package@v1.0-badversion'}       | ${pg.prepare}
  ${'404 with fetch button'} | ${'/github.com/package/does/not/exist'} | ${pg.prepare}
  ${'home'}                  | ${'/'}                                  | ${pg.prepare}
  ${'license policy'}        | ${'/license-policy'}                    | ${pg.prepare}
  ${'search'}                | ${'/search?q=http'}                     | ${search.prepare}
  ${'search help'}           | ${'/search-help'}                       | ${pg.prepare}
`;
