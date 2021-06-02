/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */const snippetEls=document.querySelectorAll(".js-toolsCopySnippet");snippetEls.forEach(t=>{t.addEventListener("click",e=>{e.preventDefault(),e.currentTarget?.select(),document.execCommand("copy")})});
//# sourceMappingURL=badge.js.map
