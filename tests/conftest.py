import pytest
from playwright.sync_api import sync_playwright


@pytest.fixture(scope="function")
def page():
    """Local Playwright page fixture for screenshot/integration tests.

    This avoids requiring pytest-playwright plugin wiring while keeping
    test_integration.py unchanged.
    """
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
