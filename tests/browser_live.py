"""Browser regression checks. Requires Playwright and its Chromium browser.
Run via make test-browser; all feed, database, and ruling data is temporary.
"""
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]


def write_json(path, data):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(data))
    temporary.replace(path)


with tempfile.TemporaryDirectory(prefix='hold-court-live-test-') as tmp:
    tmp = Path(tmp)
    feed = tmp / 'feed'
    rulings = tmp / 'rulings'
    feed.mkdir()
    rulings.mkdir()
    title = 'A long actual pull request title explaining a subtle process-group cancellation race across foreground children'
    hold = dict(id='example-42-head', repo='example/widgets', pr=42,
                title='example/widgets #42: ' + title, question='Should this behavior change?',
                review_body_md='Prepared review\n\n' + ('Context paragraph.\n\n' * 40),
                head_sha='a' * 40, held_at='2026-09-01T12:00:00Z',
                url='https://github.com/example/widgets/pull/42', **{'class': 'scope'})
    write_json(feed / 'one.json', hold)
    server = subprocess.Popen([str(ROOT / 'hold-court'), 'serve', '-feed', str(feed),
                               '-rulings', str(rulings), '-db', str(tmp / 'state.db')],
                              cwd=tmp, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                              text=True, start_new_session=True)
    try:
        url = server.stdout.readline().strip().split('session: ', 1)[1]
        with sync_playwright() as pw:
            browser = pw.chromium.launch(executable_path=os.environ.get('HOLD_COURT_CHROMIUM'))
            page = browser.new_page(viewport={'width': 1280, 'height': 900})
            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            page.goto(url)
            expect(page.locator('.hold-title')).to_have_text(title)
            list_box = page.locator('#pane-list').bounding_box()
            read_box = page.locator('#pane-reading').bounding_box()
            assert list_box['width'] > 900 and read_box['y'] >= list_box['y'] + list_box['height']
            assert page.locator('.hold-title').evaluate('(el) => getComputedStyle(el.parentElement).whiteSpace') == 'normal'
            page.locator('#pane-list li').click()
            page.locator('#note-input').fill('Keep my reasoning while other work arrives.')
            page.locator('#note-input').focus()
            page.locator('#pane-reading').evaluate('(el) => el.scrollTop = 160')
            original_scroll = page.locator('#pane-reading').evaluate('(el) => el.scrollTop')
            write_json(feed / 'two.json', dict(hold, id='example-43-head', pr=43, title='Another new hold', held_at='2026-09-02T12:00:00Z'))
            expect(page.locator('#pane-list li')).to_have_count(2, timeout=15000)
            expect(page.locator('#pane-reading h1')).to_have_text(title)
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            expect(page.locator('#note-input')).to_be_focused()
            assert page.locator('#pane-reading').evaluate('(el) => el.scrollTop') == original_scroll
            expect(page.locator('#activity-button')).to_have_text('Updates (1)')
            hold['question'] = 'Updated review question'
            write_json(feed / 'one.json', hold)
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            expect(page.locator('.question')).to_have_text('Should this behavior change?')
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.locator('#show-update').click()
            expect(page.locator('.question')).to_have_text('Updated review question')
            page.locator('[data-action="discuss"]').click()
            page.reload()
            page.locator('[data-hold-id="example-42-head"]').click()
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            expect(page.locator('#pending-bar')).to_contain_text('1 pending ruling')
            page.route('**/api/rulings', lambda route: route.fulfill(status=503, body='temporarily unavailable'))
            page.locator('#save-btn').click()
            expect(page.locator('#notice')).to_contain_text('Save failed')
            expect(page.locator('#pending-bar')).to_contain_text('1 pending ruling')
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.unroute('**/api/rulings')
            page.locator('#save-btn').click()
            expect(page.locator('#notice')).to_contain_text('Decisions saved')
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('.saved-ruling')).to_contain_text('Keep my reasoning')
            expect(page.locator('#note-input')).to_have_value('')
            assert json.loads((rulings / 'example-42-head.json').read_text())['action'] == 'discuss'
            write_json(rulings / 'example-42-head.result.json', {'status': 'failed', 'summary': 'Head changed. No action taken.'})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('.saved-ruling').last).to_contain_text('Head changed. No action taken.')
            expect(page.locator('.saved-ruling').last).to_contain_text('failed')
            for index in range(8):
                write_json(feed / f'more-{index}.json', dict(hold, id=f'more-{index}', pr=100+index,
                           title=f'Additional hold {index}', held_at='2026-09-03T12:00:00Z'))
            expect(page.locator('#pane-list li')).to_have_count(10, timeout=15000)
            page.locator('#pane-list').evaluate('(el) => el.scrollTop = 0')
            page.keyboard.press('G')
            assert page.locator('#pane-list').evaluate('(el) => el.scrollTop') > 0
            assert not errors, errors
            if os.environ.get('HOLD_COURT_SCREENSHOT'):
                page.screenshot(path=os.environ['HOLD_COURT_SCREENSHOT'])
            browser.close()
        print('PASS: stacked titles, live arrivals, preserved context/drafts, saved decisions, and result activity')
    finally:
        os.killpg(server.pid, signal.SIGINT)
        server.wait(timeout=15)
