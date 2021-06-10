# End-to-End (E2E) Tests

This directory contains end-to-end tests for pages on pkg.go.dev.

## Running E2E Tests

In order to run the tests, run this command from the root of the repository:

```
$ ./e2e/docker/run.sh
```

`./e2e/docker/run.sh` sets up a series of docker containers that run a postgres
database, frontend, and headless chrome, and runs the e2e tests using headless
chrome.

Alternatively, you can run the tests against a website that is already running.

First run headless chrome:

    docker run --rm -e "CONNECTION_TIMEOUT=-1" -p 3000:3000 browserless/chrome:1.46-chrome-stable

Then run the tests from the root of pkgsite:

    ./all.bash npx jest [files]

`PKGSITE_URL` can https://pkg.go.dev, or http://localhost:8080 if you have a
local instance for the frontend running.

### Understanding Test Failures

If the tests failure, diffs will be created that show the cause of the failure.
Timeouts and diff thresholds are configurable for image snapshots if
adjustments are needed to prevent test flakiness. See the
[API](https://github.com/americanexpress/jest-image-snapshot#%EF%B8%8F-api) for
jest image snapshots for more information.

### Writing E2E Tests

Tests are written in the Jest framework using Puppeteer to drive a headless
instance of Chrome.

Familiarize yourself with the
[Page](https://pptr.dev/#?product=Puppeteer&version=v5.5.0&show=api-class-page)
class from the Puppeteer documenation. You'll find methods on this class that
let you to interact with the page.

Most tests will follow a similar structure but for details on the Jest
framework and the various hooks and assertions see the
[API](https://jestjs.io/docs/en/api).
