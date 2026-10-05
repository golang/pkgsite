/**
 * @license
 * Copyright 2026 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { registerSurveyBanner } from './survey';

const KEY = 'go-survey-test';

function render(): HTMLElement {
  document.body.innerHTML = `
    <section class="go-SurveyBanner js-surveyBanner" data-survey-id="${KEY}">
      <a class="js-surveyBannerLink" href="https://example.com/survey">Survey</a>
      <button class="js-surveyBannerClose">x</button>
    </section>`;
  return document.querySelector('.js-surveyBanner') as HTMLElement;
}

describe('registerSurveyBanner', () => {
  afterEach(() => {
    localStorage.clear();
    document.cookie = 'cookie-consent=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
    document.body.innerHTML = '';
  });

  it('shows the banner when not minimized', () => {
    const banner = render();
    registerSurveyBanner();
    expect(banner.isConnected).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--visible')).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--minimized')).toBe(false);
  });

  it('minimizes the banner when previously minimized', () => {
    localStorage.setItem(KEY, 'minimized');
    const banner = render();
    registerSurveyBanner();
    expect(banner.isConnected).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--visible')).toBe(false);
    expect(banner.classList.contains('go-SurveyBanner--minimized')).toBe(true);
  });

  it('records minimized state and minimizes on close', () => {
    const banner = render();
    registerSurveyBanner();
    (banner.querySelector('.js-surveyBannerClose') as HTMLElement).click();
    expect(banner.isConnected).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--visible')).toBe(false);
    expect(banner.classList.contains('go-SurveyBanner--minimized')).toBe(true);
    expect(localStorage.getItem(KEY)).toBe('minimized');
  });

  it('records minimized state and minimizes when the survey link is followed', () => {
    const banner = render();
    registerSurveyBanner();
    const link = banner.querySelector('.js-surveyBannerLink') as HTMLAnchorElement;
    link.addEventListener('click', e => e.preventDefault()); // jsdom: no navigation
    link.click();
    expect(banner.isConnected).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--visible')).toBe(false);
    expect(banner.classList.contains('go-SurveyBanner--minimized')).toBe(true);
    expect(localStorage.getItem(KEY)).toBe('minimized');
  });

  it('offsets the banner above the cookie notice until acknowledged', () => {
    const banner = render();
    const cookieNotice = document.createElement('section');
    cookieNotice.className = 'Cookie-notice js-cookieNotice';
    const okButton = document.createElement('button');
    cookieNotice.appendChild(okButton);
    document.body.appendChild(cookieNotice);
    registerSurveyBanner();
    expect(banner.isConnected).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--visible')).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--aboveCookie')).toBe(true);
    okButton.click();
    expect(banner.classList.contains('go-SurveyBanner--aboveCookie')).toBe(false);
  });

  it('does not offset the banner when cookie consent has been granted', () => {
    document.cookie = 'cookie-consent=true';
    const banner = render();
    const cookieNotice = document.createElement('section');
    cookieNotice.className = 'Cookie-notice js-cookieNotice';
    document.body.appendChild(cookieNotice);
    registerSurveyBanner();
    expect(banner.isConnected).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--visible')).toBe(true);
    expect(banner.classList.contains('go-SurveyBanner--aboveCookie')).toBe(false);
  });

  it('removes the banner when data-survey-id is missing', () => {
    document.body.innerHTML = '<section class="go-SurveyBanner js-surveyBanner"></section>';
    const banner = document.querySelector('.js-surveyBanner') as HTMLElement;
    registerSurveyBanner();
    expect(banner.isConnected).toBe(false);
  });

  it('does nothing without a banner', () => {
    document.body.innerHTML = '';
    expect(() => registerSurveyBanner()).not.toThrow();
  });
});
