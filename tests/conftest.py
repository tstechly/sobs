from typing import Any

import pytest

sync_playwright: Any | None

try:
    from playwright.sync_api import sync_playwright as _real_sync_playwright

    sync_playwright = _real_sync_playwright
except ModuleNotFoundError:  # pragma: no cover - depends on environment
    sync_playwright = None


@pytest.fixture(scope="function")
def page():
    """Local Playwright page fixture for screenshot/integration tests.

    This avoids requiring pytest-playwright plugin wiring while keeping
    test_integration.py unchanged.
    """
    if sync_playwright is None:
        pytest.skip("Playwright is not installed in this test environment")

    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
        except Exception as exc:  # pragma: no cover - environment dependent
            pytest.skip(f"Playwright Chromium unavailable: {exc}")
        context = browser.new_context(viewport={"width": 1440, "height": 900})
        page_obj = context.new_page()
        try:
            yield page_obj
        finally:
            context.close()
            browser.close()
