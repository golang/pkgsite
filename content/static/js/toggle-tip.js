/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */const toggles=document.querySelectorAll("[data-toggletip-content]");toggles.forEach(t=>{const i=t.getAttribute("data-toggletip-content"),e=t.nextElementSibling;t.addEventListener("click",()=>{!e||(e.innerHTML="",setTimeout(()=>{e.innerHTML='<span class="UnitMetaDetails-toggletipBubble">'+i+"</span>"},100))}),document.addEventListener("click",n=>{if(t!==n.target){if(!e)return;e.innerHTML=""}}),t.addEventListener("keydown",n=>{!e||n.key==="Escape"&&(e.innerHTML="")})});
//# sourceMappingURL=toggle-tip.js.map
