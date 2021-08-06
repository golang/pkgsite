(()=>{var a=document.querySelector(".js-fetchButton");a&&a.addEventListener("click",e=>{e.preventDefault(),l()});async function l(){let e=document.querySelector(".js-fetchMessage"),t=document.querySelector(".js-fetchMessageSecondary"),o=document.querySelector(".js-fetchButton"),n=document.querySelector(".js-fetchLoading");if(!(e&&t&&o&&n))return;e.textContent=`Fetching ${e.dataset.path}`,t.textContent="Feel free to navigate away and check back later, we\u2019ll keep working on it!",o.style.display="none",n.style.display="block";let c=await fetch(`/fetch${window.location.pathname}`,{method:"POST"});if(c.ok){window.location.reload();return}let s=await c.text();n.style.display="none",t.textContent="";let r=new DOMParser().parseFromString(s,"text/html");e.innerHTML=r.documentElement.textContent??""}})();
/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=fetch.js.map
