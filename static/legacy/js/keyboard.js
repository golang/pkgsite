/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */const canonicalURLPath=document.querySelector(".js-canonicalURLPath")?.dataset.canonicalUrlPath;canonicalURLPath&&canonicalURLPath!==""&&document.addEventListener("keydown",t=>{const e=t.target?.tagName;if(!(e==="INPUT"||e==="SELECT"||e==="TEXTAREA")&&!t.target?.isContentEditable&&!(t.metaKey||t.ctrlKey))switch(t.key){case"y":window.history.replaceState(null,"",canonicalURLPath);break}});
//# sourceMappingURL=keyboard.js.map
