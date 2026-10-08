/**
 * @license
 * Copyright 2026 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const MINIMIZED = 'minimized';

let onHashChange: (() => void) | null = null;

/**
 * registerSurveyBanner shows the Go Developer Survey banner. On desktop,
 * clicking the minimize button or the survey link collapses the floating card
 * into a compact sticky pill ('minimized'). On mobile, a top banner is shown.
 * When the cookie notice is active, the desktop card sits above it.
 *
 * The state is recorded in localStorage, keyed by the survey ID from the
 * banner's data-survey-id attribute, so no cookie is involved. localStorage
 * is per origin, so state on pkg.go.dev does not carry over to go.dev.
 */
export function registerSurveyBanner(): void {
  if (onHashChange) {
    window.removeEventListener('hashchange', onHashChange);
    onHashChange = null;
  }
  const banner = document.querySelector<HTMLElement>('.js-surveyBanner');
  if (!banner) {
    return;
  }
  const key = banner.dataset.surveyId ?? '';
  if (!key) {
    banner.remove();
    return;
  }
  const checkReset = (): boolean => {
    if (window.location.pathname === '/' && window.location.hash === '#survey-reset') {
      clearState(key);
      window.history.replaceState(null, '', window.location.pathname + window.location.search);
      return true;
    }
    return false;
  };
  const cookieNotice = document.querySelector('.js-cookieNotice');
  if (cookieNotice && !document.cookie.match(/cookie-consent=true/)) {
    banner.classList.add('go-SurveyBanner--aboveCookie');
    cookieNotice.querySelector('button')?.addEventListener('click', () => {
      banner.classList.remove('go-SurveyBanner--aboveCookie');
    });
  }
  const minimize = () => {
    setState(key, MINIMIZED);
    banner.classList.remove('go-SurveyBanner--visible');
    banner.classList.add('go-SurveyBanner--minimized');
  };
  banner.querySelector('.js-surveyBannerLink')?.addEventListener('click', minimize);
  banner.querySelector('.js-surveyBannerClose')?.addEventListener('click', minimize);
  const update = () => {
    const state = getState(key);
    banner.classList.toggle('go-SurveyBanner--minimized', state === MINIMIZED);
    banner.classList.toggle('go-SurveyBanner--visible', state !== MINIMIZED);
  };
  onHashChange = () => {
    if (checkReset()) {
      update();
    }
  };
  window.addEventListener('hashchange', onHashChange);
  checkReset();
  update();
}

function getState(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    // Storage is unavailable (for example, disabled by the user).
    return null;
  }
}

function setState(key: string, state: string): void {
  try {
    localStorage.setItem(key, state);
  } catch {
    // Storage is unavailable; the banner shows again on the next page.
  }
}

function clearState(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch {
    // Storage is unavailable.
  }
}
