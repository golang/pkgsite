/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */const searchInput=document.querySelector(".js-searchFocus"),canonicalURLPath=document.querySelector(".js-canonicalURLPath")?.dataset.canonicalUrlPath;document.addEventListener("keydown",e=>{const t=e.target?.tagName;if(!(t==="INPUT"||t==="SELECT"||t==="TEXTAREA")&&!e.target?.isContentEditable&&!(e.metaKey||e.ctrlKey))switch(e.key){case"y":canonicalURLPath&&canonicalURLPath!==""&&window.history.replaceState(null,"",canonicalURLPath);break;case"/":searchInput&&!window.navigator.userAgent.includes("Firefox")&&(e.preventDefault(),searchInput.focus());break}});
//# sourceMappingURL=keyboard.js.map
