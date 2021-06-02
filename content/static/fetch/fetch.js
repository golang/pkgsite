/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */const fetchButton=document.querySelector(".js-fetchButton");fetchButton&&fetchButton.addEventListener("click",e=>{e.preventDefault(),fetchPath()});async function fetchPath(){const e=document.querySelector(".js-fetchMessage"),t=document.querySelector(".js-fetchMessageSecondary"),o=document.querySelector(".js-fetchButton"),n=document.querySelector(".js-fetchLoading");if(!(e&&t&&o&&n))return;e.textContent=`Fetching ${e.dataset.path}`,t.textContent="Feel free to navigate away and check back later, we\u2019ll keep working on it!",o.style.display="none",n.style.display="block";const c=await fetch(`/fetch${window.location.pathname}`,{method:"POST"});if(c.ok){window.location.reload();return}const a=await c.text();n.style.display="none",t.textContent="";const s=new DOMParser().parseFromString(a,"text/html");e.innerHTML=s.documentElement.textContent??""}
//# sourceMappingURL=fetch.js.map
